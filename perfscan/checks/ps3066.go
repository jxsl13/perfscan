package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3066 reports three or more consecutive sibling loops over the same
// bound that all index the same buffer.
var PS3066 = register(&lint.Check{
	ID:       "PS3066",
	Category: "indirect",
	Slug:     "consecutive-loops-over-one-buffer",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "three or more sibling loops over the same bound, all touching one buffer",
		Text: `Each pass streams the whole buffer and evicts it before the next
pass starts. If every index is independent ACROSS the loops, merge them: the
buffer is then touched once and stays in cache through all the stages.
Merging is bit-identical because it changes only WHEN a row is visited,
never how — each row's stages already ran in that order relative to each
other.

Check for a cross-index dependency first: a later loop that needs ALL of an
earlier loop's output cannot merge. The win is the buffer SIZE, not the loop
count — the reference measured -15.9% on a state larger than L2 and -1.8%
on one that was already L1-resident.`,
		Before: `for i := range state { state[i] *= decay }
for i := range state { s += state[i] * k[i] }
for i := range state { state[i] += delta[i] }`,
		After: `for i := range state {
	state[i] *= decay
	s += state[i] * k[i]
	state[i] += delta[i]
}`,
		MeasuredWin: "reference corpus: Kimi delta-attention four passes merged into one, BenchmarkKDA_F64_256x128 8.71→7.33ms (-15.9%); same merge on an L1-resident 64×64 state: -1.8%",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3066",
		Doc:  "consecutive sibling loops over one buffer",
		Run:  runPS3066,
	},
})

func runPS3066(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			checkConsecutiveLoops(pass, f, block)
			return true
		})
	}
	return nil, nil
}

type loopInfo struct {
	stmt    ast.Stmt
	bound   string
	indexed map[string]bool
}

func checkConsecutiveLoops(pass *analysis.Pass, f *ast.File, block *ast.BlockStmt) {
	var run []loopInfo
	flush := func() {
		defer func() { run = run[:0] }()
		if len(run) < 3 {
			return
		}
		// Common buffer indexed by every loop in the run.
		common := make(map[string]bool, len(run[0].indexed))
		for b := range run[0].indexed {
			common[b] = true
		}
		for _, li := range run[1:] {
			for b := range common {
				if !li.indexed[b] {
					delete(common, b)
				}
			}
		}
		if len(common) == 0 {
			return
		}
		names := make([]string, 0, len(common))
		for b := range common {
			names = append(names, b)
		}
		stmts := make([]ast.Stmt, len(run))
		for i, li := range run {
			stmts[i] = li.stmt
		}
		diag := analysis.Diagnostic{
			Pos:     run[0].stmt.Pos(),
			End:     run[0].stmt.End(),
			Message: fmt.Sprintf("%d consecutive loops over the same bound stream %s once each; if the per-index work is independent across the loops, merge them so the buffer stays in cache (a later loop needing ALL of an earlier one's output cannot merge)", len(run), strings.Join(names, ", ")),
		}
		if fix := ps3066Fix(pass, f, stmts); fix != nil {
			diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
		}
		pass.Report(diag)
	}
	for _, stmt := range block.List {
		bound, indexed, ok := loopSignature(stmt)
		if !ok {
			flush()
			continue
		}
		if len(run) > 0 && run[len(run)-1].bound != bound {
			flush()
		}
		run = append(run, loopInfo{stmt: stmt, bound: bound, indexed: indexed})
	}
	flush()
}

// loopSignature renders a loop's bound and collects the identifiers it
// indexes.
func loopSignature(s ast.Stmt) (string, map[string]bool, bool) {
	var bound ast.Expr
	var body *ast.BlockStmt
	switch l := s.(type) {
	case *ast.RangeStmt:
		bound = l.X
		body = l.Body
	case *ast.ForStmt:
		if l.Cond == nil {
			return "", nil, false
		}
		bound = l.Cond
		body = l.Body
	default:
		return "", nil, false
	}
	var b strings.Builder
	_ = printer.Fprint(&b, token.NewFileSet(), bound)
	indexed := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if ix, ok := n.(*ast.IndexExpr); ok {
			if id, ok := ix.X.(*ast.Ident); ok {
				indexed[id.Name] = true
			}
		}
		return true
	})
	return b.String(), indexed, true
}

// ps3066Fix builds the loop-merge rewrite for the exact provably safe shape
// and nothing else:
//
//   - every loop in the run is `for IV := range BUF` (`:=`, no value var,
//     the same loop-variable name, the same plain-ident BUF of slice or
//     array type), and
//   - every body is built from a small whitelist (assignments, ++/--, if,
//     nested blocks, var declarations) in which the ONLY element access is
//     BUF[IV], the only calls are builtin len/cap, no scalar is written in
//     one loop and mentioned in another, no top-level declaration of one
//     body is mentioned in a sibling body, and no operation can panic
//     (integer / and % need a nonzero constant divisor, shifts a constant
//     count).
//
// Under those gates the bodies cannot panic, BUF's header is never
// reassigned, and every per-index stage still runs in its original order
// relative to the other stages of the same index, so concatenating the
// bodies into one loop is bit-identical. Distinct slice identifiers can
// alias each other at an offset, which is why any access to a second
// buffer declines the fix (the diagnostic stays advisory). The edits
// delete only the whole lines holding each interior `}` / `for … {` pair,
// so body text and its comments survive verbatim.
func ps3066Fix(pass *analysis.Pass, f *ast.File, stmts []ast.Stmt) *analysis.SuggestedFix {
	info := pass.TypesInfo
	if info == nil || len(stmts) < 2 {
		return nil
	}
	loops := make([]*ast.RangeStmt, 0, len(stmts))
	for _, s := range stmts {
		rs, ok := s.(*ast.RangeStmt)
		if !ok {
			return nil
		}
		loops = append(loops, rs)
	}
	key0, ok := loops[0].Key.(*ast.Ident)
	if !ok || key0.Name == "_" {
		return nil
	}
	src0, ok := loops[0].X.(*ast.Ident)
	if !ok {
		return nil
	}
	bufObj := info.ObjectOf(src0)
	if _, isVar := bufObj.(*types.Var); !isVar {
		return nil
	}
	switch bufObj.Type().Underlying().(type) {
	case *types.Slice, *types.Array:
	default:
		return nil
	}
	for _, l := range loops {
		if l.Tok != token.DEFINE || l.Value != nil || len(l.Body.List) == 0 {
			return nil
		}
		k, ok := l.Key.(*ast.Ident)
		if !ok || k.Name != key0.Name {
			return nil
		}
		x, ok := l.X.(*ast.Ident)
		if !ok || info.ObjectOf(x) != bufObj {
			return nil
		}
	}
	// Scan each body against the whitelist, collecting per-loop facts.
	scans := make([]*ps3066BodyScan, len(loops))
	for k, l := range loops {
		ivObj := info.ObjectOf(l.Key.(*ast.Ident))
		if ivObj == nil {
			return nil
		}
		sc := &ps3066BodyScan{
			info:     info,
			buf:      bufObj,
			iv:       ivObj,
			mentions: map[types.Object]bool{},
			writes:   map[types.Object]bool{},
		}
		if !sc.stmts(l.Body.List, true) {
			return nil
		}
		scans[k] = sc
	}
	// Cross-loop dependence gates: a scalar written by one loop and
	// mentioned by another would see partial results after the merge; a
	// name declared at a body's top level would capture or collide when
	// the bodies share one block scope.
	for j := range scans {
		for k := range scans {
			if j == k {
				continue
			}
			for obj := range scans[j].writes {
				if scans[k].mentions[obj] {
					return nil
				}
			}
			for _, name := range scans[j].declared {
				if stmtsMention(loops[k].Body.List, name) {
					return nil
				}
			}
		}
	}
	// Formatting gates: each interior `}` and each following `for … {`
	// header must occupy whole lines of their own so the edits can delete
	// exactly those lines, and no comment may sit inside a deleted range.
	tf := pass.Fset.File(loops[0].Pos())
	if tf == nil {
		return nil
	}
	edits := make([]analysis.TextEdit, 0, len(loops)-1)
	for k := 0; k+1 < len(loops); k++ {
		cur, next := loops[k], loops[k+1]
		last := cur.Body.List[len(cur.Body.List)-1]
		lineRb := tf.Line(cur.Body.Rbrace)
		if tf.Line(last.End()) >= lineRb {
			return nil
		}
		lineFor := tf.Line(next.Pos())
		lineLb := tf.Line(next.Body.Lbrace)
		if lineRb >= lineFor || lineFor != lineLb {
			return nil
		}
		if tf.Line(next.Body.List[0].Pos()) <= lineLb || lineLb+1 > tf.LineCount() {
			return nil
		}
		delStart, delEnd := tf.LineStart(lineRb), tf.LineStart(lineLb+1)
		for _, cg := range f.Comments {
			if cg.End() > delStart && cg.Pos() < delEnd {
				return nil
			}
		}
		edits = append(edits, analysis.TextEdit{Pos: delStart, End: delEnd})
	}
	return &analysis.SuggestedFix{
		Message:   fmt.Sprintf("Merge the %d loops over %s into one loop", len(loops), src0.Name),
		TextEdits: edits,
	}
}

// ps3066BodyScan vets one loop body for the mergeable-run fix and records
// which outer objects it mentions and writes.
type ps3066BodyScan struct {
	info     *types.Info
	buf      types.Object // the shared range source
	iv       types.Object // this loop's own index variable
	mentions map[types.Object]bool
	writes   map[types.Object]bool
	declared []string // names declared at the body's top statement level
}

func (sc *ps3066BodyScan) stmts(list []ast.Stmt, top bool) bool {
	for _, s := range list {
		if !sc.stmt(s, top) {
			return false
		}
	}
	return true
}

func (sc *ps3066BodyScan) stmt(s ast.Stmt, top bool) bool {
	switch st := s.(type) {
	case *ast.AssignStmt:
		return sc.assign(st, top)
	case *ast.IncDecStmt:
		return sc.target(st.X)
	case *ast.IfStmt:
		if st.Init != nil && !sc.stmt(st.Init, false) {
			return false
		}
		if !sc.expr(st.Cond) || !sc.stmts(st.Body.List, false) {
			return false
		}
		switch e := st.Else.(type) {
		case nil:
			return true
		case *ast.BlockStmt:
			return sc.stmts(e.List, false)
		case *ast.IfStmt:
			return sc.stmt(e, false)
		}
		return false
	case *ast.BlockStmt:
		return sc.stmts(st.List, false)
	case *ast.DeclStmt:
		gd, ok := st.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			return false
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				return false
			}
			if vs.Type != nil {
				if _, ok := vs.Type.(*ast.Ident); !ok {
					return false
				}
			}
			for _, v := range vs.Values {
				if !sc.expr(v) {
					return false
				}
			}
			for _, n := range vs.Names {
				if top {
					sc.declared = append(sc.declared, n.Name)
				}
				if obj := sc.info.ObjectOf(n); obj != nil {
					sc.writes[obj] = true
					sc.mentions[obj] = true
				}
			}
		}
		return true
	case *ast.EmptyStmt:
		return true
	}
	// Anything else (calls, control flow, defer/go, send, …) makes the
	// merge unprovable.
	return false
}

func (sc *ps3066BodyScan) assign(st *ast.AssignStmt, top bool) bool {
	if st.Tok == token.DEFINE {
		for _, lhs := range st.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok {
				return false
			}
			if top {
				sc.declared = append(sc.declared, id.Name)
			}
			if id.Name == "_" {
				continue
			}
			obj := sc.info.ObjectOf(id)
			if obj == nil || obj == sc.iv || obj == sc.buf {
				return false
			}
			if _, ok := obj.(*types.Var); !ok {
				return false
			}
			sc.writes[obj] = true
			sc.mentions[obj] = true
		}
	} else {
		var opTok token.Token
		switch st.Tok {
		case token.ASSIGN, token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN,
			token.AND_ASSIGN, token.OR_ASSIGN, token.XOR_ASSIGN, token.AND_NOT_ASSIGN:
		case token.QUO_ASSIGN:
			opTok = token.QUO
		case token.REM_ASSIGN:
			opTok = token.REM
		case token.SHL_ASSIGN:
			opTok = token.SHL
		case token.SHR_ASSIGN:
			opTok = token.SHR
		default:
			return false
		}
		if opTok != token.ILLEGAL {
			if len(st.Lhs) != 1 || len(st.Rhs) != 1 ||
				!sc.opSafe(opTok, sc.info.TypeOf(st.Lhs[0]), st.Rhs[0]) {
				return false
			}
		}
		for _, lhs := range st.Lhs {
			if !sc.target(lhs) {
				return false
			}
		}
	}
	for _, rhs := range st.Rhs {
		if !sc.expr(rhs) {
			return false
		}
	}
	return true
}

// target vets an assignment / ++ / -- destination: a plain non-buffer,
// non-index scalar identifier or BUF[IV].
func (sc *ps3066BodyScan) target(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.Ident:
		if t.Name == "_" {
			return true
		}
		obj := sc.info.ObjectOf(t)
		if obj == nil || obj == sc.iv || obj == sc.buf {
			return false
		}
		if _, ok := obj.(*types.Var); !ok {
			return false
		}
		sc.writes[obj] = true
		sc.mentions[obj] = true
		return true
	case *ast.IndexExpr:
		return sc.bufIndex(t)
	}
	return false
}

// bufIndex accepts exactly BUF[IV].
func (sc *ps3066BodyScan) bufIndex(ix *ast.IndexExpr) bool {
	base, ok := ix.X.(*ast.Ident)
	if !ok || sc.info.ObjectOf(base) != sc.buf {
		return false
	}
	idx, ok := ix.Index.(*ast.Ident)
	return ok && sc.info.ObjectOf(idx) == sc.iv
}

func (sc *ps3066BodyScan) expr(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return sc.readIdent(x)
	case *ast.BasicLit:
		return true
	case *ast.ParenExpr:
		return sc.expr(x.X)
	case *ast.BinaryExpr:
		switch x.Op {
		case token.QUO, token.REM, token.SHL, token.SHR:
			if !sc.opSafe(x.Op, sc.info.TypeOf(x), x.Y) {
				return false
			}
		}
		return sc.expr(x.X) && sc.expr(x.Y)
	case *ast.UnaryExpr:
		switch x.Op {
		case token.ADD, token.SUB, token.XOR, token.NOT:
			return sc.expr(x.X)
		}
		return false // & takes an address, <- receives
	case *ast.IndexExpr:
		return sc.bufIndex(x)
	case *ast.CallExpr:
		fn, ok := x.Fun.(*ast.Ident)
		if !ok || x.Ellipsis != token.NoPos || len(x.Args) != 1 {
			return false
		}
		b, ok := sc.info.ObjectOf(fn).(*types.Builtin)
		if !ok || (b.Name() != "len" && b.Name() != "cap") {
			return false
		}
		arg, ok := x.Args[0].(*ast.Ident)
		if !ok {
			return false
		}
		if sc.info.ObjectOf(arg) == sc.buf {
			return true
		}
		return sc.readIdent(arg)
	}
	return false
}

func (sc *ps3066BodyScan) readIdent(id *ast.Ident) bool {
	obj := sc.info.ObjectOf(id)
	if obj == nil || obj == sc.buf {
		// A bare use of BUF outside BUF[IV] / len / cap could rebind or
		// leak the buffer.
		return false
	}
	if obj == sc.iv {
		return true
	}
	switch obj.(type) {
	case *types.Var:
		sc.mentions[obj] = true
		return true
	case *types.Const, *types.Nil:
		return true
	}
	return false
}

// opSafe vets a division, remainder, or shift for panic-freedom: float and
// complex division cannot panic, integer division and remainder need a
// nonzero constant divisor, and shifts need a constant count (constant
// counts are compile-checked non-negative).
func (sc *ps3066BodyScan) opSafe(op token.Token, typ types.Type, rhs ast.Expr) bool {
	if op == token.QUO && typ != nil {
		if b, ok := typ.Underlying().(*types.Basic); ok && b.Info()&(types.IsFloat|types.IsComplex) != 0 {
			return true
		}
	}
	tv, ok := sc.info.Types[rhs]
	if !ok || tv.Value == nil {
		return false
	}
	switch op {
	case token.QUO, token.REM:
		return tv.Value.Kind() != constant.Unknown && constant.Sign(tv.Value) != 0
	case token.SHL, token.SHR:
		return true
	}
	return false
}
