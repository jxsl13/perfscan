package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS4008 reports a matmul-shaped nest whose innermost loop is a serial
// scalar dot accumulator.
var PS4008 = register(&lint.Check{
	ID:       "PS4008",
	Category: "vector",
	Slug:     "serial-dot-matmul",
	Level:    lint.LevelAggressive,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a matmul whose innermost loop is a serial scalar dot accumulator",
		Text: `sum += a[…]*b[…] in the innermost loop of a ≥3-deep nest chains
every fused multiply-add on the previous one: the loop runs at FMA latency,
not throughput. An ikj/axpy loop order (or a small block of independent
accumulators) breaks the dependency chain and lets the core retire several
FMAs per cycle.

L3 (aggressive): reassociating a floating-point reduction changes the
rounding order, so the result is NOT bit-identical. Gate the rewrite with a
tolerance-based oracle or restructure so each output element keeps one
accumulation order (the ikj form does: each c[i][j] still sums k ascending).

The automatic fix rewrites only the canonical shape below, where a, b, c
are distinct [][]float64 variables: it zeroes the output row first (the
original overwrote c[i][j], so the rewritten accumulation must start from
the same +0.0), then accumulates rank-1 updates with k ascending. Each
c[i][j] therefore sees the identical IEEE addition sequence and the result
is bit-identical — unless c shares a backing array with a or b (in-place
matmul), which the fix cannot prove absent; review the fix where they
could alias.`,
		Before: `for i := range a {
	for j := range b[0] {
		sum := 0.0
		for k := range b {
			sum += a[i][k] * b[k][j] // latency chain
		}
		c[i][j] = sum
	}
}`,
		After: `for i := range a {
	for j := range b[0] {
		c[i][j] = 0
	}
	for k := range b { // axpy: independent accumulators per j
		for j := range b[0] {
			c[i][j] += a[i][k] * b[k][j]
		}
	}
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS4008",
		Doc:  "serial scalar dot accumulator in a matmul nest",
		Run:  runPS4008,
	},
})

func runPS4008(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			if !astutil.IsLoop(n) {
				return true
			}
			body := astutil.LoopBody(n)
			if body == nil || containsLoop(body) {
				return true
			}
			// Depth ≥3: at least two enclosing loops.
			depth := 1
			for _, anc := range stack {
				if astutil.IsLoop(anc) {
					depth++
				}
			}
			if depth < 3 {
				return true
			}
			if !hasSerialDotAccum(body) {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     n.Pos(),
				End:     body.Lbrace,
				Message: "innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain (reassociation is not bit-identical — gate with a tolerance oracle)",
			}
			if fix := ps4008AxpyFix(pass, stack, n); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

func containsLoop(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		if n != body && astutil.IsLoop(n) {
			found = true
			return false
		}
		return true
	})
	return found
}

// hasSerialDotAccum matches `s += a[…] * b[…]` with a plain scalar target.
func hasSerialDotAccum(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		if _, ok := as.Lhs[0].(*ast.Ident); !ok {
			return true
		}
		mul, ok := as.Rhs[0].(*ast.BinaryExpr)
		if !ok || mul.Op != token.MUL {
			return true
		}
		_, xIdx := mul.X.(*ast.IndexExpr)
		_, yIdx := mul.Y.(*ast.IndexExpr)
		if xIdx && yIdx {
			found = true
			return false
		}
		return true
	})
	return found
}

// ps4008AxpyFix builds the ikj/axpy rewrite for the exact canonical shape
//
//	for J := range B[0] {
//		SUM := 0.0
//		for K := range B {
//			SUM += A[I][K] * B[K][J]
//		}
//		C[I][J] = SUM
//	}
//
// where A, B, C are (underlying) [][]float64 slice variables, C is not the
// same variable as A or B, and I is an identifier distinct from J and K.
// The replacement
//
//	for J := range B[0] {
//		C[I][J] = 0
//	}
//	for K := range B {
//		for J := range B[0] {
//			C[I][J] += A[I][K] * B[K][J]
//		}
//	}
//
// keeps every c[I][J] accumulating the same terms from the same +0.0 in the
// same ascending-k order, so each output element's IEEE rounding sequence is
// unchanged (bit-identical absent aliasing between C and A/B). inner is the
// innermost (k) loop the diagnostic was reported on.
func ps4008AxpyFix(pass *analysis.Pass, stack []ast.Node, inner ast.Node) *analysis.SuggestedFix {
	innerLoop, ok := inner.(*ast.RangeStmt)
	if !ok {
		return nil
	}
	mid, ok := astutil.InLoop(stack)
	if !ok {
		return nil
	}
	middle, ok := mid.(*ast.RangeStmt)
	if !ok {
		return nil
	}
	// A labeled middle loop would leave its label attached to only the
	// zeroing loop after the rewrite; skip.
	for idx := len(stack) - 1; idx >= 1; idx-- {
		if stack[idx] == ast.Node(middle) {
			if _, isLab := stack[idx-1].(*ast.LabeledStmt); isLab {
				return nil
			}
			break
		}
	}
	info := pass.TypesInfo

	// Middle header: for J := range B[0], with J a used (non-blank) int var.
	if middle.Tok != token.DEFINE || middle.Value != nil {
		return nil
	}
	jIdent, ok := middle.Key.(*ast.Ident)
	if !ok || jIdent.Name == "_" {
		return nil
	}
	bRow, ok := middle.X.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	bIdent, ok := bRow.X.(*ast.Ident)
	if !ok {
		return nil
	}
	zero, ok := bRow.Index.(*ast.BasicLit)
	if !ok || zero.Kind != token.INT || zero.Value != "0" {
		return nil
	}
	if len(middle.Body.List) != 3 {
		return nil
	}

	// Statement 1: SUM := 0.0 (any float literal spelling of +0).
	initStmt, ok := middle.Body.List[0].(*ast.AssignStmt)
	if !ok || initStmt.Tok != token.DEFINE || len(initStmt.Lhs) != 1 || len(initStmt.Rhs) != 1 {
		return nil
	}
	sumIdent, ok := initStmt.Lhs[0].(*ast.Ident)
	if !ok || sumIdent.Name == "_" {
		return nil
	}
	sumLit, ok := initStmt.Rhs[0].(*ast.BasicLit)
	if !ok || sumLit.Kind != token.FLOAT {
		return nil
	}
	if v, err := strconv.ParseFloat(sumLit.Value, 64); err != nil || v != 0 {
		return nil
	}

	// Statement 2: the reported inner loop itself, for K := range B.
	if s, ok := middle.Body.List[1].(*ast.RangeStmt); !ok || s != innerLoop {
		return nil
	}
	if innerLoop.Tok != token.DEFINE || innerLoop.Value != nil {
		return nil
	}
	kIdent, ok := innerLoop.Key.(*ast.Ident)
	if !ok || kIdent.Name == "_" {
		return nil
	}
	bIdent2, ok := innerLoop.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if len(innerLoop.Body.List) != 1 {
		return nil
	}

	jObj := info.ObjectOf(jIdent)
	kObj := info.ObjectOf(kIdent)
	sumObj := info.ObjectOf(sumIdent)
	bObj := info.ObjectOf(bIdent)
	if jObj == nil || kObj == nil || sumObj == nil || bObj == nil {
		return nil
	}
	if info.ObjectOf(bIdent2) != bObj {
		return nil
	}

	// Inner body: SUM += A[I][K] * B[K][J].
	acc, ok := innerLoop.Body.List[0].(*ast.AssignStmt)
	if !ok || acc.Tok != token.ADD_ASSIGN || len(acc.Lhs) != 1 || len(acc.Rhs) != 1 {
		return nil
	}
	accLhs, ok := acc.Lhs[0].(*ast.Ident)
	if !ok || info.ObjectOf(accLhs) != sumObj {
		return nil
	}
	mul, ok := acc.Rhs[0].(*ast.BinaryExpr)
	if !ok || mul.Op != token.MUL {
		return nil
	}
	aIdent, iIdent, k1, ok := ps4008MatrixIndex(mul.X)
	if !ok {
		return nil
	}
	b3, k2, j1, ok := ps4008MatrixIndex(mul.Y)
	if !ok {
		return nil
	}
	if info.ObjectOf(k1) != kObj || info.ObjectOf(k2) != kObj || info.ObjectOf(j1) != jObj {
		return nil
	}
	if info.ObjectOf(b3) != bObj {
		return nil
	}
	iObj := info.ObjectOf(iIdent)
	if iObj == nil || iObj == jObj || iObj == kObj || iObj == sumObj {
		return nil
	}

	// Statement 3: C[I][J] = SUM.
	store, ok := middle.Body.List[2].(*ast.AssignStmt)
	if !ok || store.Tok != token.ASSIGN || len(store.Lhs) != 1 || len(store.Rhs) != 1 {
		return nil
	}
	cIdent, i2, j2, ok := ps4008MatrixIndex(store.Lhs[0])
	if !ok {
		return nil
	}
	if info.ObjectOf(i2) != iObj || info.ObjectOf(j2) != jObj {
		return nil
	}
	storeRhs, ok := store.Rhs[0].(*ast.Ident)
	if !ok || info.ObjectOf(storeRhs) != sumObj {
		return nil
	}
	cObj := info.ObjectOf(cIdent)
	aObj := info.ObjectOf(aIdent)
	if cObj == nil || aObj == nil {
		return nil
	}
	// In-place matmul (c literally a or b) reads elements it already
	// overwrote; the rewrite would change which values are read.
	if cObj == aObj || cObj == bObj {
		return nil
	}
	// The rewrite moves C (and keeps A, B, I) inside the j/k loop bodies;
	// a name collision with the loop variables would capture them.
	for _, name := range []string{aIdent.Name, bIdent.Name, cIdent.Name, iIdent.Name} {
		if name == jIdent.Name || name == kIdent.Name {
			return nil
		}
	}

	// All three matrices must be (underlying) slices of slices of float64:
	// slice ranging is deterministic ascending (maps are not), and the
	// element type is the exact type of the 0.0 accumulator.
	for _, id := range []*ast.Ident{aIdent, bIdent, cIdent} {
		if !ps4008IsFloat64Matrix(info.TypeOf(id)) {
			return nil
		}
	}

	col := pass.Fset.Position(middle.Pos()).Column
	if col < 1 {
		return nil
	}
	// Assume gofmt indentation (tabs), as ps2005 does.
	indent := strings.Repeat("\t", col-1)
	a, b, c, i, j, k := aIdent.Name, bIdent.Name, cIdent.Name, iIdent.Name, jIdent.Name, kIdent.Name
	var sb strings.Builder
	fmt.Fprintf(&sb, "for %s := range %s[0] {\n", j, b)
	fmt.Fprintf(&sb, "%s\t%s[%s][%s] = 0\n", indent, c, i, j)
	fmt.Fprintf(&sb, "%s}\n", indent)
	fmt.Fprintf(&sb, "%sfor %s := range %s {\n", indent, k, b)
	fmt.Fprintf(&sb, "%s\tfor %s := range %s[0] {\n", indent, j, b)
	fmt.Fprintf(&sb, "%s\t\t%s[%s][%s] += %s[%s][%s] * %s[%s][%s]\n", indent, c, i, j, a, i, k, b, k, j)
	fmt.Fprintf(&sb, "%s\t}\n", indent)
	fmt.Fprintf(&sb, "%s}", indent)
	return &analysis.SuggestedFix{
		Message: "restructure to ikj/axpy order: zero the output row, then accumulate rank-1 updates (per-element accumulation order preserved)",
		TextEdits: []analysis.TextEdit{
			{Pos: middle.Pos(), End: middle.End(), NewText: []byte(sb.String())},
		},
	}
}

// ps4008MatrixIndex unpacks BASE[ROW][COL] where all three are plain
// identifiers.
func ps4008MatrixIndex(e ast.Expr) (base, row, col *ast.Ident, ok bool) {
	outer, ok2 := e.(*ast.IndexExpr)
	if !ok2 {
		return nil, nil, nil, false
	}
	col, ok2 = outer.Index.(*ast.Ident)
	if !ok2 {
		return nil, nil, nil, false
	}
	in, ok2 := outer.X.(*ast.IndexExpr)
	if !ok2 {
		return nil, nil, nil, false
	}
	row, ok2 = in.Index.(*ast.Ident)
	if !ok2 {
		return nil, nil, nil, false
	}
	base, ok2 = in.X.(*ast.Ident)
	if !ok2 {
		return nil, nil, nil, false
	}
	return base, row, col, true
}

// ps4008IsFloat64Matrix reports whether t is (underlying) a slice whose
// element is (underlying) a slice of exactly float64.
func ps4008IsFloat64Matrix(t types.Type) bool {
	if t == nil {
		return false
	}
	outer, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	in, ok := outer.Elem().Underlying().(*types.Slice)
	if !ok {
		return false
	}
	bt, ok := in.Elem().(*types.Basic)
	return ok && bt.Kind() == types.Float64
}
