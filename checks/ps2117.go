package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2117 reports a local `k := string(b)` ([]byte operand) whose every use
// is a map lookup key — the variable defeats the compiler's allocation-free
// m[string(b)] map-index special case.
var PS2117 = register(&lint.Check{
	ID:       "PS2117",
	Category: "alloc",
	Slug:     "map-key-conversion-var",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string([]byte) bound to a variable used only as a map key defeats the compiler's allocation-free lookup",
		Text: `The compiler special-cases a []byte→string conversion written
INLINE in a map index or delete — m[string(b)], delete(m, string(b)) — into
a temporary string header sharing b's bytes: no allocation, no copy. The
special case is syntactic. Binding the conversion to a variable first
(k := string(b); ... m[k]) generally defeats it and materializes a real
string, even when k is never used for anything but map lookups. Recent
compilers (observed on go1.26) recover the very simplest variable shape —
one lookup immediately after the declaration — but with more than one
keyed use, or any statement in between, the variable form allocates
(above the 32-byte non-escaping stack-buffer threshold), while the
inline spelling is elided on every toolchain.

The remedy is to inline the conversion at each index: m[string(b)]. Only
declarations whose EVERY use is a pure map-key read (m[k] as a value,
including the comma-ok form) or a delete(m, k) key are reported — a map
WRITE m[k] = v genuinely needs the materialized string, and any other use
keeps the variable alive, so those are not flagged at all.

Inlining moves the conversion from the declaration point to each use, so
it is only bit-identical when b's value and b's backing bytes provably
cannot change in between. The automatic fix is attached only under those
proofs: the conversion is spelled with the predeclared string type over a
plain identifier; all uses sit in sibling statements after the declaration
in the same block, outside any function literal; the statements up to the
last use contain no calls (other than the matched deletes), no writes
through an index or pointer, no writes to b or k, no address-of b or k, no
channel operations, goroutine launches, defers, or labels (a label could
be jumped to from below, re-entering the window after a mutation); b and
the identifier "string" still resolve to the same objects at every use;
and the declaration sits alone on its line so the line can be deleted
cleanly (a trailing line comment is removed with the statement it
documents; any other overlapping comment suppresses the fix). Everything
else — a call, closure, or possible
mutation between declaration and use — keeps the plain advisory report:
apply the inline rewrite manually after confirming nothing can mutate b's
bytes between the two points.

Value parity of the fix itself: string(b) of an unchanged b yields an
equal string at every use (map lookup compares values, not identities),
nil and empty b both convert to "", the conversion cannot panic, and no
argument evaluation is reordered inside the guarded window.`,
		Before: `k := string(b)
if v, ok := hits[k]; ok {
	return v
}
return misses[k]`,
		After: `if v, ok := hits[string(b)]; ok {
	return v
}
return misses[string(b)]`,
		MeasuredWin: `BenchmarkPS2117 (1024 iterations, two lookups of a
~55-byte key in a 1024-entry map[string]int, Apple M2 Pro, go1.26):
35.3 µs/op, 1024 allocs/op, 65536 B/op -> 21.8 µs/op, 0 allocs/op
(~1.6x, every key allocation eliminated; the win grows with key size
and GC pressure).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2117",
		Doc:  "string([]byte) stored in a variable used only as a map key allocates; inline m[string(b)] does not",
		Run:  runPS2117,
	},
})

func runPS2117(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				return true
			}
			kIdent, ok := as.Lhs[0].(*ast.Ident)
			if !ok || kIdent.Name == "_" {
				return true
			}
			bIdent, ok := ps2117StringOfBytes(pass, as.Rhs[0])
			if !ok {
				return true
			}
			kObj, ok := pass.TypesInfo.Defs[kIdent].(*types.Var)
			if !ok {
				return true
			}
			body := ps2117EnclosingBody(stack)
			if body == nil {
				return true
			}
			uses := ps2117KeyOnlyUses(pass, body, kObj)
			if uses == nil {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     as.Pos(),
				End:     as.End(),
				Message: kIdent.Name + " := string(" + bIdent.Name + ") allocates a string used only as a map key; the compiler elides the allocation when the conversion is inline in the index: m[string(" + bIdent.Name + ")]",
			}
			if fix := ps2117Fix(pass, f, stack, as, bIdent, uses); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2117StringOfBytes matches rhs as string(b) where string is the
// predeclared type (not shadowed, not a named string type — a named target
// would not compile against a map[string]V index anyway, and only the
// canonical spelling is reproduced by the fix) and b is a plain identifier
// whose type's underlying is a slice of the predeclared byte.
func ps2117StringOfBytes(pass *analysis.Pass, rhs ast.Expr) (*ast.Ident, bool) {
	call, ok := rhs.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, false
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[fun] != types.Universe.Lookup("string") {
		return nil, false
	}
	arg, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return nil, false
	}
	if _, isVar := pass.TypesInfo.Uses[arg].(*types.Var); !isVar {
		return nil, false
	}
	t := pass.TypesInfo.TypeOf(arg)
	if t == nil {
		return nil, false
	}
	sl, ok := t.Underlying().(*types.Slice)
	if !ok {
		return nil, false
	}
	eb, ok := sl.Elem().(*types.Basic)
	if !ok || eb.Kind() != types.Byte {
		return nil, false
	}
	return arg, true
}

// ps2117EnclosingBody returns the body of the innermost enclosing function
// (declaration or literal) on the stack.
func ps2117EnclosingBody(stack []ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncDecl:
			return fn.Body
		case *ast.FuncLit:
			return fn.Body
		}
	}
	return nil
}

// ps2117Use is one accepted use of the key variable: the identifier, its
// ancestor stack within the enclosing function body (innermost last), and
// the delete call when the use is a delete(m, k) key.
type ps2117Use struct {
	id    *ast.Ident
	stack []ast.Node
	del   *ast.CallExpr
}

// ps2117KeyOnlyUses collects every use of obj within body and returns them
// only when ALL of them are pure map-key positions: the index of a map READ
// (an r-value m[k], including the comma-ok form — never an assignment
// target or inc/dec operand) with key type exactly string, or the key
// argument of builtin delete on such a map. Any other use returns nil.
func ps2117KeyOnlyUses(pass *analysis.Pass, body *ast.BlockStmt, obj *types.Var) []ps2117Use {
	var uses []ps2117Use
	bad := false
	astutil.WithStack(body, func(n ast.Node, stack []ast.Node) bool {
		if bad {
			return false
		}
		id, ok := n.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[id] != types.Object(obj) {
			return true
		}
		if len(stack) == 0 {
			bad = true
			return false
		}
		switch p := stack[len(stack)-1].(type) {
		case *ast.IndexExpr:
			if p.Index != ast.Expr(id) || !ps2117StringKeyMap(pass, p.X) || ps2117IndexWritten(stack, p) {
				bad = true
				return false
			}
			uses = append(uses, ps2117Use{id: id, stack: slices.Clone(stack)})
		case *ast.CallExpr:
			fn, ok := p.Fun.(*ast.Ident)
			if !ok || len(p.Args) != 2 || p.Args[1] != ast.Expr(id) {
				bad = true
				return false
			}
			if bi, isB := pass.TypesInfo.Uses[fn].(*types.Builtin); !isB || bi.Name() != "delete" {
				bad = true
				return false
			}
			if !ps2117StringKeyMap(pass, p.Args[0]) {
				bad = true
				return false
			}
			uses = append(uses, ps2117Use{id: id, stack: slices.Clone(stack), del: p})
		default:
			bad = true
			return false
		}
		return true
	})
	if bad || len(uses) == 0 {
		return nil
	}
	return uses
}

// ps2117StringKeyMap reports whether e's type is (underlying) a map whose
// key type is exactly the predeclared string — the shape the runtime's
// byte-slice-keyed fast path applies to.
func ps2117StringKeyMap(pass *analysis.Pass, e ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return false
	}
	m, ok := t.Underlying().(*types.Map)
	return ok && types.Identical(m.Key(), types.Typ[types.String])
}

// ps2117IndexWritten reports whether the map index expression ix is a write
// target: an assignment (plain or compound) left-hand side or an inc/dec
// operand. A written entry needs the materialized key string.
func ps2117IndexWritten(stack []ast.Node, ix *ast.IndexExpr) bool {
	if len(stack) < 2 {
		return false
	}
	switch gp := stack[len(stack)-2].(type) {
	case *ast.AssignStmt:
		for _, lhs := range gp.Lhs {
			if lhs == ast.Expr(ix) {
				return true
			}
		}
	case *ast.IncDecStmt:
		return gp.X == ast.Expr(ix)
	}
	return false
}

// ps2117Fix builds the inline rewrite — replace every m[k] / delete(m, k)
// key with string(b) and delete the k declaration line — when it is
// provably bit-identical, or returns nil to keep the advisory report.
func ps2117Fix(pass *analysis.Pass, f *ast.File, declStack []ast.Node, as *ast.AssignStmt, b *ast.Ident, uses []ps2117Use) *analysis.SuggestedFix {
	if len(declStack) == 0 {
		return nil
	}
	block, ok := declStack[len(declStack)-1].(*ast.BlockStmt)
	if !ok {
		return nil
	}
	di := -1
	for i, s := range block.List {
		if s == ast.Stmt(as) {
			di = i
			break
		}
	}
	if di < 0 || di+1 >= len(block.List) {
		return nil
	}
	bObj, ok := pass.TypesInfo.Uses[b].(*types.Var)
	if !ok {
		return nil
	}
	kObj, _ := pass.TypesInfo.Defs[as.Lhs[0].(*ast.Ident)].(*types.Var)
	universeString := types.Universe.Lookup("string")

	// Every use must sit in a sibling statement after the declaration, with
	// no function literal between (a closure runs at an unknowable time),
	// and both b and "string" must still resolve to the same objects there.
	deleteCalls := make(map[*ast.CallExpr]bool, len(uses))
	lastIdx := di
	for _, u := range uses {
		ci, ok := ps2117Container(block, u.stack)
		if !ok || ci <= di {
			return nil
		}
		if ci > lastIdx {
			lastIdx = ci
		}
		if u.del != nil {
			deleteCalls[u.del] = true
		}
		scope := pass.Pkg.Scope().Innermost(u.id.Pos())
		if scope == nil {
			return nil
		}
		if _, o := scope.LookupParent(b.Name, u.id.Pos()); o != types.Object(bObj) {
			return nil
		}
		if _, o := scope.LookupParent("string", u.id.Pos()); o != universeString {
			return nil
		}
	}

	// The statements from the declaration to the last use must be inert:
	// nothing in them may mutate b's variable or backing bytes, reassign k,
	// or synchronize with another goroutine.
	for _, s := range block.List[di+1 : lastIdx+1] {
		if !ps2117Inert(pass, s, bObj, kObj, deleteCalls) {
			return nil
		}
	}

	// The declaration must sit alone on its line so deleting the whole line
	// leaves no residue (a trailing line comment goes with it).
	tf := pass.Fset.File(as.Pos())
	if tf == nil {
		return nil
	}
	declLine := tf.Line(as.Pos())
	if tf.Line(as.End()) != declLine || declLine+1 > tf.LineCount() {
		return nil
	}
	if tf.Line(block.Lbrace) == declLine {
		return nil
	}
	if di > 0 && tf.Line(block.List[di-1].End()) == declLine {
		return nil
	}
	if tf.Line(block.List[di+1].Pos()) == declLine {
		return nil
	}
	lineStart := tf.LineStart(declLine)
	nextStart := tf.LineStart(declLine + 1)
	for _, cg := range f.Comments {
		if cg.End() <= lineStart || cg.Pos() >= nextStart {
			continue // elsewhere in the file
		}
		// A trailing line comment documents the deleted statement and goes
		// with it; any other overlap (a comment before or inside the
		// statement, or one spanning past the line) suppresses the fix.
		if cg.Pos() > as.End() && cg.End() < nextStart {
			continue
		}
		return nil
	}

	repl := "string(" + b.Name + ")"
	// The replacement text is identical at every key-use site, so build the
	// []byte once above the loop instead of re-converting per iteration — the
	// loop-invariant hoist PS3101 flags, dogfooded here. The analysis framework
	// treats a TextEdit's NewText as read-only, so sharing one slice across the
	// edits is safe.
	replBytes := []byte(repl)
	edits := []analysis.TextEdit{{Pos: lineStart, End: nextStart}}
	for _, u := range uses {
		edits = append(edits, analysis.TextEdit{Pos: u.id.Pos(), End: u.id.End(), NewText: replBytes})
	}
	return &analysis.SuggestedFix{
		Message:   "inline the conversion at each map index: " + repl,
		TextEdits: edits,
	}
}

// ps2117Container returns the index in block.List of the statement that
// contains the use (whose ancestor stack is given), and whether no function
// literal sits between that statement and the use.
func ps2117Container(block *ast.BlockStmt, stack []ast.Node) (int, bool) {
	bi := -1
	for i, n := range stack {
		if n == ast.Node(block) {
			bi = i
			break
		}
	}
	if bi < 0 || bi+1 >= len(stack) {
		return 0, false
	}
	for _, n := range stack[bi+1:] {
		if _, isLit := n.(*ast.FuncLit); isLit {
			return 0, false
		}
	}
	st, ok := stack[bi+1].(ast.Stmt)
	if !ok {
		return 0, false
	}
	for j, s := range block.List {
		if s == st {
			return j, true
		}
	}
	return 0, false
}

// ps2117Inert reports whether statement s provably cannot mutate b's value
// or backing bytes, reassign k, or synchronize with another goroutine.
// Rejected anywhere in the subtree:
//   - any call except the matched delete(m, k) uses (an arbitrary call may
//     reach b's array through an alias; even a type conversion is rejected
//     for simplicity),
//   - go/defer/send/select/receive and range over a channel or function
//     (synchronization or deferred execution windows),
//   - a label (a goto from below the window could re-enter it after a
//     mutation),
//   - writes through an index or pointer (b's bytes may alias them),
//     writes to b or k themselves, or taking their address.
func ps2117Inert(pass *analysis.Pass, s ast.Stmt, bObj, kObj *types.Var, deleteCalls map[*ast.CallExpr]bool) bool {
	inert := true
	ast.Inspect(s, func(n ast.Node) bool {
		if !inert {
			return false
		}
		switch nd := n.(type) {
		case *ast.CallExpr:
			if !deleteCalls[nd] {
				inert = false
			}
		case *ast.GoStmt, *ast.DeferStmt, *ast.SendStmt, *ast.SelectStmt, *ast.LabeledStmt:
			inert = false
		case *ast.UnaryExpr:
			switch nd.Op {
			case token.ARROW:
				inert = false
			case token.AND:
				if id, ok := nd.X.(*ast.Ident); ok && ps2117IsObj(pass, id, bObj, kObj) {
					inert = false
				}
			}
		case *ast.AssignStmt:
			for _, lhs := range nd.Lhs {
				if ps2117WriteTarget(pass, lhs, bObj, kObj) {
					inert = false
				}
			}
		case *ast.IncDecStmt:
			if ps2117WriteTarget(pass, nd.X, bObj, kObj) {
				inert = false
			}
		case *ast.RangeStmt:
			if ps2117WriteTarget(pass, nd.Key, bObj, kObj) || ps2117WriteTarget(pass, nd.Value, bObj, kObj) {
				inert = false
			}
			if t := pass.TypesInfo.TypeOf(nd.X); t != nil {
				switch t.Underlying().(type) {
				case *types.Chan, *types.Signature:
					inert = false
				}
			}
		}
		return inert
	})
	return inert
}

// ps2117WriteTarget reports whether assigning to e could affect b's bytes
// or k's value: a write through any index or pointer dereference (it may
// alias b's backing array), or a write to b or k directly.
func ps2117WriteTarget(pass *analysis.Pass, e ast.Expr, bObj, kObj *types.Var) bool {
	switch t := e.(type) {
	case nil:
		return false
	case *ast.IndexExpr, *ast.StarExpr:
		return true
	case *ast.Ident:
		return ps2117IsObj(pass, t, bObj, kObj)
	}
	return false
}

// ps2117IsObj reports whether id resolves to bObj or kObj.
func ps2117IsObj(pass *analysis.Pass, id *ast.Ident, bObj, kObj *types.Var) bool {
	o := pass.TypesInfo.Uses[id]
	return o == types.Object(bObj) || (kObj != nil && o == types.Object(kObj))
}
