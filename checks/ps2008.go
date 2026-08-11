package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2008 reports loops that allocate one slice per iteration into
// ARR[loopvar] with a loop-invariant length — a single slab plus disjoint
// capped views would do.
var PS2008 = register(&lint.Check{
	ID:       "PS2008",
	Category: "alloc",
	Slug:     "per-row-make-slab",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "a loop that allocates one uniform slice per iteration into ARR[i]",
		Text: `A loop filling ARR[i] = make([]T, L) with a loop-invariant L
performs one allocation per row where a single slab would do: allocate
make([]T, n*L) once and cut three-index views
ARR[i] = slab[i*L : (i+1)*L : (i+1)*L]. Bit-identical by construction —
make() zeroes and so does a fresh slab.

Preconditions the check cannot verify: the rows must die together (a slab
pins every row in it), must not be appended to (the 3-index cap makes an
append copy instead of corrupting the neighbor, but the benefit is then
lost), and must not be individually replaced later.

Expect an allocation-count win always and a wall-clock win only when the
loop body is small relative to the allocator work; measure the enclosing
operation. The payoff scales with the iteration count, which is a runtime
fact — prefer sites whose loop bound is a data size over ones bounded by a
small constant.`,
		Before: `rows := make([][]float64, n)
for i := range rows {
	rows[i] = make([]float64, d)
}`,
		After: `rows := make([][]float64, n)
slab := make([]float64, n*d)
for i := range rows {
	rows[i] = slab[i*d : (i+1)*d : (i+1)*d]
}`,
		MeasuredWin: "reference corpus: classic DBSCAN block-allocator variant -66.18%/-74.13% allocs/op, -5.90%/-7.48% time (p=0.000, n=12); GMM PredictProba rows -11.17% at k=4 alongside allocs 605→94",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2008",
		Doc:  "per-row make with invariant length into ARR[loopvar]",
		Run:  runPS2008,
	},
})

func runPS2008(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			loopVar, body := loopVarAndBody(n)
			if loopVar == "" || body == nil {
				return true
			}
			for _, stmt := range body.List {
				as, ok := stmt.(*ast.AssignStmt)
				if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					continue
				}
				idx, ok := as.Lhs[0].(*ast.IndexExpr)
				if !ok {
					continue
				}
				arr, ok := idx.X.(*ast.Ident)
				if !ok {
					continue
				}
				iv, ok := idx.Index.(*ast.Ident)
				if !ok || iv.Name != loopVar {
					continue
				}
				mk, ok := as.Rhs[0].(*ast.CallExpr)
				if !ok || astutil.CalleeName(mk.Fun) != "make" || len(mk.Args) < 2 {
					continue
				}
				if _, isSlice := mk.Args[0].(*ast.ArrayType); !isSlice {
					continue
				}
				if mentionsIdent(mk.Args[1], loopVar) {
					continue
				}
				pass.Report(analysis.Diagnostic{
					Pos:     as.Pos(),
					End:     as.End(),
					Message: arr.Name + "[" + loopVar + "] gets its own make per iteration with a loop-invariant length; one slab plus 3-index views cuts the allocations to one (rows must die together and not be appended to)",
				})
			}
			return true
		})
	}
	return nil, nil
}

// loopVarAndBody extracts the integer loop variable and body of
// `for i := ...` / `for i := range xs`.
func loopVarAndBody(n ast.Node) (string, *ast.BlockStmt) {
	switch l := n.(type) {
	case *ast.ForStmt:
		if as, ok := l.Init.(*ast.AssignStmt); ok && len(as.Lhs) == 1 {
			if id, ok := as.Lhs[0].(*ast.Ident); ok {
				return id.Name, l.Body
			}
		}
	case *ast.RangeStmt:
		if id, ok := l.Key.(*ast.Ident); ok && id.Name != "_" {
			return id.Name, l.Body
		}
	}
	return "", nil
}

func mentionsIdent(e ast.Expr, name string) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}
