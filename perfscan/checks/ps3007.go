package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3007 reports a membership set (map[K]bool / map[K]struct{}) built by
// ranging a slice and then probed inside a loop.
var PS3007 = register(&lint.Check{
	ID:       "PS3007",
	Category: "indirect",
	Slug:     "set-map-from-slice",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a membership set built from a slice the caller already owns, probed in a loop",
		Text: `When a set's contents come from a slice the code already holds,
the fix is often no map at all: scanning the source slice directly beats the
map build plus one hash per probe as long as the source stays small (the
reference corpus measured the crossover at 8–16 elements on an M2 Pro). This
is a SMALL-SET transform — large sets should keep the map.

Two narrowings keep the check honest, both inherited from the reference:
the set must be read-only after its build loop (a mutable working set
genuinely needs a map), and a build already guarded by a size threshold on
the source is silent — that code has taken this advice and kept the map as
its large-set fallback. An emptiness guard (len(src) > 0) is not a
threshold.

Hotness is not visible to the AST: confirm the source is small and the
probe repeats, then benchmark.

The auto-fix handles only the fully local shape: a make(map[K]bool[, size])
or map[K]bool{} declaration immediately followed by its build loop over a
local slice, where every other use of the set is a plain boolean probe
SET[k] after the build (no comma-ok, no len, never passed on, none inside a
closure) and "slices" resolves to the standard library at each probe. The
declaration and build loop are deleted and each probe becomes
slices.Contains(SRC, k).

The key type must additionally be STRICTLY comparable — an interface key
(or a key containing an interface anywhere in its structure) stays advisory.
With an interface key holding an uncomparable dynamic value (a slice, map,
or func), the map build inserts every element and panics at BUILD time,
while slices.Contains compares lazily and panics only if the scan actually
reaches that element at PROBE time — never, when no probe runs or a match
comes first. That is a panic-timing behavior divergence, so such sets keep
the map.`,
		Before: `breakers := make(map[int64]bool, len(seq))
for _, b := range seq {
	breakers[b] = true
}
for _, tok := range window {
	if breakers[tok] { ... }
}`,
		After: `for _, tok := range window {
	if slices.Contains(seq, tok) { ... } // seq is small
}`,
		MeasuredWin: "reference corpus: nlp applyDRY -18.72% (19.52µs→15.87µs, p=0.002) with runtime.mapaccess1_fast64 leaving the profile entirely",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3007",
		Doc:  "membership set built from an owned slice, probed in a loop",
		Run:  runPS3007,
	},
})

func runPS3007(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checkSetMapFunc(pass, f, fn)
		}
	}
	return nil, nil
}

func isSetMapType(t types.Type) bool {
	m, ok := t.Underlying().(*types.Map)
	if !ok {
		return false
	}
	switch v := m.Elem().Underlying().(type) {
	case *types.Basic:
		return v.Kind() == types.Bool
	case *types.Struct:
		return v.NumFields() == 0
	}
	return false
}

func checkSetMapFunc(pass *analysis.Pass, f *ast.File, fn *ast.FuncDecl) {
	// Builds of the shape `for _, v := range SRC { SET[v] = ... }` with a
	// single-statement body, where SET has a set-shaped map type.
	type build struct {
		loop *ast.RangeStmt
		src  ast.Expr
	}
	builds := map[string]build{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		r, ok := n.(*ast.RangeStmt)
		if !ok || r.Value == nil || r.Body == nil || len(r.Body.List) != 1 {
			return true
		}
		v, ok := r.Value.(*ast.Ident)
		if !ok {
			return true
		}
		as, ok := r.Body.List[0].(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 {
			return true
		}
		idx, ok := as.Lhs[0].(*ast.IndexExpr)
		if !ok {
			return true
		}
		m, ok := idx.X.(*ast.Ident)
		if !ok {
			return true
		}
		k, ok := idx.Index.(*ast.Ident)
		if !ok || k.Name != v.Name {
			return true
		}
		t := pass.TypesInfo.TypeOf(idx.X)
		if t == nil || !isSetMapType(t) {
			return true
		}
		// The range source must be slice-typed (an owned scan target).
		if st := pass.TypesInfo.TypeOf(r.X); st == nil {
			return true
		} else if _, ok := st.Underlying().(*types.Slice); !ok {
			return true
		}
		builds[m.Name] = build{loop: r, src: r.X}
		return true
	})
	if len(builds) == 0 {
		return
	}

	inBuild := func(name string, n ast.Node) bool {
		b, ok := builds[name]
		return ok && n.Pos() >= b.loop.Pos() && n.End() <= b.loop.End()
	}

	// Narrowing 1: drop sets written outside their build loop.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, l := range as.Lhs {
			ix, ok := l.(*ast.IndexExpr)
			if !ok {
				continue
			}
			m, ok := ix.X.(*ast.Ident)
			if !ok {
				continue
			}
			if _, tracked := builds[m.Name]; tracked && !inBuild(m.Name, ix) {
				delete(builds, m.Name)
			}
		}
		return true
	})

	// Narrowing 2: drop builds guarded by a size threshold on the source
	// (already-taken advice). Emptiness guards (compare against 0) do not
	// count.
	astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
		r, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		for name, b := range builds {
			if b.loop != r {
				continue
			}
			for _, anc := range stack {
				ifs, ok := anc.(*ast.IfStmt)
				if ok && condHasSizeThreshold(ifs.Cond) {
					delete(builds, name)
					break
				}
			}
		}
		return true
	})
	if len(builds) == 0 {
		return
	}

	// Names of every surviving set: a probe key mentioning one of these
	// could nest another rewrite's edit range inside this one, so the fix
	// declines on such keys.
	setNames := make([]string, 0, len(builds))
	for name := range builds {
		setNames = append(setNames, name)
	}

	// Probes: reads of the set inside a loop, outside the build.
	reported := map[string]bool{}
	astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
		idx, ok := n.(*ast.IndexExpr)
		if !ok {
			return true
		}
		m, ok := idx.X.(*ast.Ident)
		if !ok || reported[m.Name] {
			return true
		}
		b, tracked := builds[m.Name]
		if !tracked || inBuild(m.Name, idx) {
			return true
		}
		// Skip assignment targets (writes were already handled, but a
		// comma-ok read must be distinguished from a write target).
		if len(stack) > 0 {
			if as, ok := stack[len(stack)-1].(*ast.AssignStmt); ok && as.Tok == token.ASSIGN {
				if slices.Contains(as.Lhs, ast.Expr(idx)) {
					return true
				}
			}
		}
		if _, inLoop := astutil.InLoop(stack); !inLoop {
			return true
		}
		reported[m.Name] = true
		src := exprString(b.src)
		diag := analysis.Diagnostic{
			Pos:     idx.Pos(),
			End:     idx.End(),
			Message: "set " + m.Name + " is built from slice " + src + " and only probed; for a small source, scanning " + src + " directly beats the map build plus a hash per probe (crossover ≈8–16 elements)",
		}
		if fix := ps3007Fix(pass, f, fn, b.loop, m, setNames); fix != nil {
			diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
		}
		pass.Report(diag)
		return true
	})
}

// condHasSizeThreshold reports whether cond compares a len(...) against a
// nonzero integer literal.
func condHasSizeThreshold(cond ast.Expr) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		switch be.Op {
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
		default:
			return true
		}
		isLen := func(e ast.Expr) bool {
			c, ok := e.(*ast.CallExpr)
			return ok && astutil.CalleeName(c.Fun) == "len"
		}
		nonzeroLit := func(e ast.Expr) bool {
			l, ok := e.(*ast.BasicLit)
			if !ok || l.Kind != token.INT {
				return false
			}
			v, err := strconv.Atoi(l.Value)
			return err == nil && v != 0
		}
		if (isLen(be.X) && nonzeroLit(be.Y)) || (isLen(be.Y) && nonzeroLit(be.X)) {
			found = true
			return false
		}
		return true
	})
	return found
}

func exprString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprString(x.X) + "." + x.Sel.Name
	}
	return "the source"
}

// ps3007Fix builds the direct-scan rewrite for the one provably safe shape
// and nothing else:
//
//	SET := make(map[K]bool[, size]) // or SET := map[K]bool{}
//	for _, v := range SRC {         // immediately follows the decl
//		SET[v] = true
//	}
//
// where SRC is a plain function-local slice ident that is only ever ranged
// by the build or read via builtin len/cap (never written, indexed, sliced,
// aliased, or passed anywhere), the map key type is identical to SRC's
// element type, the built value is the constant true, and every use of SET
// outside its declaration and build is a plain boolean probe SET[k]
// positioned after the build — no comma-ok, no writes, never passed on, and
// none inside a function literal (a closure may outlive the function while
// the caller mutates SRC, breaking the snapshot equivalence). Under those
// conditions SET[k] answers exactly "k == some element of SRC" under the
// same == that slices.Contains uses (including NaN keys, which neither side
// ever matches), so the declaration and build loop are deleted whole-line
// and each probe becomes slices.Contains(SRC, k). The fix additionally
// requires "slices" to resolve to the standard-library package and SRC to
// be unshadowed at every probe, and declines when a comment sits inside any
// edited range. Any deviation returns nil and the diagnostic stays
// advisory.
func ps3007Fix(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl, loop *ast.RangeStmt, probeSet *ast.Ident, setNames []string) *analysis.SuggestedFix {
	info := pass.TypesInfo
	setObj, ok := info.Uses[probeSet].(*types.Var)
	if !ok || setObj.Pos() < fn.Pos() || setObj.Pos() > fn.End() {
		return nil
	}
	srcIdent, ok := loop.X.(*ast.Ident)
	if !ok {
		return nil
	}
	srcObj, ok := info.Uses[srcIdent].(*types.Var)
	if !ok || srcObj.Pos() < fn.Pos() || srcObj.Pos() > fn.End() {
		return nil
	}
	srcName := srcIdent.Name

	// Bool-valued set only (struct{} sets probe via comma-ok, which has no
	// expression-for-expression replacement), key type identical to the
	// source's element type.
	mt, ok := setObj.Type().Underlying().(*types.Map)
	if !ok {
		return nil
	}
	if vb, ok := mt.Elem().Underlying().(*types.Basic); !ok || vb.Kind() != types.Bool {
		return nil
	}
	st, ok := srcObj.Type().Underlying().(*types.Slice)
	if !ok || !types.Identical(mt.Key(), st.Elem()) {
		return nil
	}
	// The key must be STRICTLY comparable: a comparison that can panic (an
	// interface key, or a key containing one, holding an uncomparable dynamic
	// value) makes the two forms diverge on panic TIMING — the build loop
	// inserts every element and panics at build time, while slices.Contains
	// compares lazily and panics only if the scan actually reaches that
	// element (never, when no probe runs or a match comes first).
	if ps3007MayPanicOnCompare(mt.Key()) {
		return nil
	}

	// Re-verify the build loop by object identity: for _, v := range SRC
	// { SET[v] = true } with a constant-true value.
	if loop.Tok != token.DEFINE || loop.Body == nil || len(loop.Body.List) != 1 {
		return nil
	}
	vIdent, ok := loop.Value.(*ast.Ident)
	if !ok {
		return nil
	}
	vObj := info.Defs[vIdent]
	if vObj == nil {
		return nil
	}
	bas, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || bas.Tok != token.ASSIGN || len(bas.Lhs) != 1 || len(bas.Rhs) != 1 {
		return nil
	}
	bIdx, ok := bas.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	bSet, ok := bIdx.X.(*ast.Ident)
	if !ok || info.Uses[bSet] != types.Object(setObj) {
		return nil
	}
	if bKey, ok := bIdx.Index.(*ast.Ident); !ok || info.Uses[bKey] != vObj {
		return nil
	}
	tv, ok := info.Types[bas.Rhs[0]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Bool || !constant.BoolVal(tv.Value) {
		return nil
	}

	// No labels (and therefore no gotos) anywhere in the function: a
	// forward goto could reach a probe without executing the build.
	labeled := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if _, isLbl := n.(*ast.LabeledStmt); isLbl {
			labeled = true
		}
		return !labeled
	})
	if labeled {
		return nil
	}

	// The declaration must immediately precede the build loop in the same
	// block, so every probe after the build is dominated by a completed
	// build (statements of a block execute in order; SET's scope confines
	// all uses to this block).
	var blk *ast.BlockStmt
	loopIdx := -1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if blk != nil {
			return false
		}
		b, isBlk := n.(*ast.BlockStmt)
		if !isBlk {
			return true
		}
		for i, s := range b.List {
			if s == ast.Stmt(loop) {
				blk, loopIdx = b, i
				return false
			}
		}
		return true
	})
	if blk == nil || loopIdx < 1 || loopIdx+1 >= len(blk.List) {
		return nil
	}
	decl, ok := blk.List[loopIdx-1].(*ast.AssignStmt)
	if !ok || decl.Tok != token.DEFINE || len(decl.Lhs) != 1 || len(decl.Rhs) != 1 {
		return nil
	}
	if dIdent, ok := decl.Lhs[0].(*ast.Ident); !ok || info.Defs[dIdent] != types.Object(setObj) {
		return nil
	}
	switch rhs := decl.Rhs[0].(type) {
	case *ast.CallExpr:
		fun, isIdent := rhs.Fun.(*ast.Ident)
		if !isIdent || fun.Name != "make" {
			return nil
		}
		if _, isBuiltin := info.Uses[fun].(*types.Builtin); !isBuiltin {
			return nil
		}
		switch len(rhs.Args) {
		case 1:
		case 2:
			if !ps3007SafeSizeArg(info, rhs.Args[1]) {
				return nil
			}
		default:
			return nil
		}
	case *ast.CompositeLit:
		if len(rhs.Elts) != 0 {
			return nil
		}
	default:
		return nil
	}

	// Every use of SET must be its declaration, the build write, or an
	// eligible probe after the build.
	okUses := true
	var probes []*ast.IndexExpr
	astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
		if !okUses {
			return false
		}
		id, isIdent := n.(*ast.Ident)
		if !isIdent || (info.Uses[id] != types.Object(setObj) && info.Defs[id] != types.Object(setObj)) {
			return true
		}
		if id.Pos() >= decl.Pos() && id.End() <= decl.End() {
			return true // the declaration itself
		}
		if id.Pos() >= loop.Pos() && id.End() <= loop.End() {
			if id != bSet {
				okUses = false
			}
			return okUses
		}
		if len(stack) == 0 {
			okUses = false
			return false
		}
		idx, isIx := stack[len(stack)-1].(*ast.IndexExpr)
		if !isIx || idx.X != ast.Expr(id) || idx.Pos() <= loop.End() {
			okUses = false
			return false
		}
		for _, anc := range stack {
			if _, isLit := anc.(*ast.FuncLit); isLit {
				okUses = false // a closure may outlive the build's snapshot
				return false
			}
		}
		if len(stack) >= 2 {
			switch p := stack[len(stack)-2].(type) {
			case *ast.AssignStmt:
				if slices.Contains(p.Lhs, ast.Expr(idx)) ||
					(len(p.Lhs) == 2 && len(p.Rhs) == 1 && p.Rhs[0] == ast.Expr(idx)) {
					okUses = false // write target or comma-ok read
					return false
				}
			case *ast.ValueSpec:
				if len(p.Names) == 2 && len(p.Values) == 1 && p.Values[0] == ast.Expr(idx) {
					okUses = false // comma-ok read in a var decl
					return false
				}
			}
		}
		probes = append(probes, idx)
		return true
	})
	if !okUses || len(probes) == 0 {
		return nil
	}
	for _, idx := range probes {
		keyText := exprTextRendered(idx.Index)
		if keyText == "" || strings.IndexByte(keyText, "\n"[0]) >= 0 {
			return nil
		}
		// A key mentioning any tracked set could nest another rewrite's
		// edit range inside this probe's replacement.
		for _, name := range setNames {
			if exprMentions(idx.Index, name) {
				return nil
			}
		}
		if !stdPkgInScope(pass, idx.Pos(), "slices") {
			return nil
		}
		scope := pass.Pkg.Scope().Innermost(idx.Pos())
		if scope == nil {
			return nil
		}
		if _, obj := scope.LookupParent(srcName, idx.Pos()); obj != types.Object(srcObj) {
			return nil
		}
	}

	// SRC must stay provably identical between build and probes: its only
	// uses are its declaration, the build loop's range source, and reads
	// via builtin len/cap. Anything else (reassignment, element writes,
	// slicing, address-taking, passing it on) declines.
	okSrc := true
	astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
		if !okSrc {
			return false
		}
		id, isIdent := n.(*ast.Ident)
		if !isIdent {
			return true
		}
		if info.Defs[id] == types.Object(srcObj) {
			return true // its declaration
		}
		if info.Uses[id] != types.Object(srcObj) {
			return true
		}
		if id == srcIdent {
			return true // the build loop's range source
		}
		if len(stack) > 0 {
			if call, isCall := stack[len(stack)-1].(*ast.CallExpr); isCall && len(call.Args) == 1 && call.Args[0] == ast.Expr(id) {
				if fun, isFun := call.Fun.(*ast.Ident); isFun && (fun.Name == "len" || fun.Name == "cap") {
					if _, isBuiltin := info.Uses[fun].(*types.Builtin); isBuiltin {
						return true
					}
				}
			}
		}
		okSrc = false
		return false
	})
	if !okSrc {
		return nil
	}

	// Whole-line deletion of decl + build: both must own their lines
	// exclusively so the removal stays gofmt-clean.
	tf := pass.Fset.File(decl.Pos())
	if tf == nil {
		return nil
	}
	declLine := tf.Line(decl.Pos())
	endLine := tf.Line(loop.End())
	if endLine >= tf.LineCount() {
		return nil
	}
	if tf.Line(blk.Lbrace) >= declLine || tf.Line(blk.Rbrace) <= endLine {
		return nil
	}
	if loopIdx >= 2 && tf.Line(blk.List[loopIdx-2].End()) >= declLine {
		return nil
	}
	if tf.Line(blk.List[loopIdx+1].Pos()) <= endLine {
		return nil
	}
	delStart := tf.LineStart(declLine)
	delEnd := tf.LineStart(endLine + 1)
	for _, cg := range file.Comments {
		if cg.End() > delStart && cg.Pos() < delEnd {
			return nil // a comment inside the deleted range would vanish
		}
		for _, idx := range probes {
			if cg.End() > idx.Pos() && cg.Pos() < idx.End() {
				return nil
			}
		}
	}

	edits := []analysis.TextEdit{{Pos: delStart, End: delEnd}}
	for _, idx := range probes {
		newText := "slices.Contains(" + srcName + ", " + exprTextRendered(idx.Index) + ")"
		edits = append(edits, analysis.TextEdit{Pos: idx.Pos(), End: idx.End(), NewText: []byte(newText)})
	}
	return &analysis.SuggestedFix{
		Message:   "replace set " + setObj.Name() + " with slices.Contains scans of " + srcName,
		TextEdits: edits,
	}
}

// ps3007MayPanicOnCompare reports whether comparing two values of type t
// with == can panic at runtime. Go comparison panics only when an interface
// value holds an uncomparable dynamic type, so any interface type — or any
// composite that CONTAINS one (a struct field, an array element) — may
// panic, while basics, pointers, and channels compare by value or identity
// and never do. The fix demands strict comparability because the map build
// inserts (and therefore compares/hashes) EVERY element eagerly, while
// slices.Contains compares lazily; on a panicking key they diverge in panic
// timing even though both are legal map-key types.
func ps3007MayPanicOnCompare(t types.Type) bool {
	return ps3007MayPanicOnCompareRec(t, make(map[types.Type]bool))
}

func ps3007MayPanicOnCompareRec(t types.Type, seen map[types.Type]bool) bool {
	if seen[t] {
		// Already being examined: a cycle cannot introduce a new panic path
		// (and comparable recursive types only recur through pointers, which
		// stop the walk anyway).
		return false
	}
	seen[t] = true
	switch u := t.Underlying().(type) {
	case *types.Interface:
		return true // may hold an uncomparable dynamic value
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if ps3007MayPanicOnCompareRec(u.Field(i).Type(), seen) {
				return true
			}
		}
		return false
	case *types.Array:
		return ps3007MayPanicOnCompareRec(u.Elem(), seen)
	case *types.Basic, *types.Pointer, *types.Chan:
		return false
	default:
		// Slices, maps, and signatures are not comparable at all and cannot
		// have reached here as a valid map key; anything else unexpected
		// (e.g. a type parameter) is treated as may-panic, declining the fix.
		return true
	}
}

// ps3007SafeSizeArg reports whether deleting the make call may drop its
// size argument unevaluated: an int literal or builtin len/cap of a plain
// identifier (never negative, no side effects). A bare variable is refused
// because a negative value would have made the original make panic.
func ps3007SafeSizeArg(info *types.Info, e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.BasicLit:
		return x.Kind == token.INT
	case *ast.CallExpr:
		fun, ok := x.Fun.(*ast.Ident)
		if !ok || (fun.Name != "len" && fun.Name != "cap") || len(x.Args) != 1 {
			return false
		}
		if _, ok := info.Uses[fun].(*types.Builtin); !ok {
			return false
		}
		_, isIdent := x.Args[0].(*ast.Ident)
		return isIdent
	}
	return false
}
