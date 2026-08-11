package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS1003 reports batch APIs called with a single-element nested slice
// literal inside a loop.
var PS1003 = register(&lint.Check{
	ID:       "PS1003",
	Category: "access",
	Slug:     "batch-single-elt",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "a batch API called with a single-element slice literal inside a loop",
		Text: `Wrapping one row in a fresh [][]T{row} per iteration to satisfy a
batch API allocates the wrapper and forfeits the batching the API exists
for. Batch the rows once outside the loop, or use the API's single-item
form if one exists.`,
		Before: `for _, row := range rows {
	model.PredictBatch([][]float64{row})
}`,
		After: `model.PredictBatch(rows)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1003",
		Doc:  "batch API with a single-element slice literal in a loop",
		Run:  runPS1003,
	},
})

// isSingleEltNestedSliceLit matches a slice literal with exactly one element
// whose element type is itself a slice/array — a wrapped row.
func isSingleEltNestedSliceLit(e ast.Expr) bool {
	cl, ok := e.(*ast.CompositeLit)
	if !ok || len(cl.Elts) != 1 {
		return false
	}
	at, ok := cl.Type.(*ast.ArrayType)
	if !ok || at.Len != nil {
		return false
	}
	_, nested := at.Elt.(*ast.ArrayType)
	return nested
}

func runPS1003(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			single := false
			for _, a := range call.Args {
				if isSingleEltNestedSliceLit(a) {
					single = true
					break
				}
			}
			if !single {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			name := astutil.CalleeName(call.Fun)
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "batch API " + name + " called with a single-element slice literal in a loop; batch the items once outside the loop or use a single-item API",
			})
			return true
		})
	}
	return nil, nil
}
