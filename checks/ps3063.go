package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3063 reports a serial nest inside a function that ALREADY fans out
// somewhere else.
var PS3063 = register(&lint.Check{
	ID:          "PS3063",
	Category:    "indirect",
	Slug:        "one-loop-left-serial-in-a-fanning-function",
	Level:       lint.LevelAggressive,
	NeedsConfig: true,
	Vocab:       []string{"fanOutHelpers"},
	Doc: lint.Documentation{
		Title: "a serial nest inside a function that already fans out elsewhere",
		Text: `A function that calls a fan-out helper has already proven the
parallel transform is available to it — and simply left one loop behind.
The reference found this shape twice, both large (6.2x and 2.3x), in files
whose OTHER hot loop had fanned out since it was written.

L3 (aggressive): check the writes own DISJOINT output before converting —
a triangular loop writing (i,j) and (j,i) does, since a clash would force
the indices equal — and gate with BOTH a bit-exact digest and -race. Which
gate catches what depends on the destination: an accumulated one (+=)
double-counts on an overlap so both fire, while a pure permutation writes
the same value twice and only -race sees it.`,
		Before: `func (l *Layer) Forward(x Tensor) {
	parallel.For(n, func(i int) { l.buildBasis(i, x) }) // fans out
	for i := 0; i < n; i++ {                            // 84% of the layer, serial
		for j := 0; j < m; j++ {
			l.fusedSpline(i, j, x)
		}
	}
}`,
		After: `func (l *Layer) Forward(x Tensor) {
	parallel.For(n, func(i int) { l.buildBasis(i, x) })
	parallel.For(n, func(i int) { // disjoint rows; digest + -race gated
		for j := 0; j < m; j++ {
			l.fusedSpline(i, j, x)
		}
	})
}`,
		MeasuredWin: "reference corpus: KAN fusedSpline 67.28→10.90ms (6.2x); eigendecomposition adjoint third product 18.63→8.17ms at n=256 (2.3x)",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3063",
		Doc:  "serial nest in a function that already fans out",
		Run:  runPS3063,
	},
})

func callsFanOut(n ast.Node, fanout map[string]bool) bool {
	found := false
	ast.Inspect(n, func(m ast.Node) bool {
		if found {
			return false
		}
		if call, ok := m.(*ast.CallExpr); ok && fanout[astutil.CalleeName(call.Fun)] {
			found = true
			return false
		}
		return true
	})
	return found
}

func runPS3063(pass *analysis.Pass) (any, error) {
	fanout := config.Current().FanOutHelpers
	if len(fanout) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !callsFanOut(fn.Body, fanout) {
				continue
			}
			// Top-level nests: a loop whose body contains another loop,
			// with no fan-out call anywhere inside.
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if !astutil.IsLoop(n) {
					return true
				}
				body := astutil.LoopBody(n)
				if body == nil || !containsLoop(body) || callsFanOut(body, fanout) {
					// Descend only when this loop is not itself a
					// reported nest root.
					return true
				}
				pass.Report(analysis.Diagnostic{
					Pos:     n.Pos(),
					End:     body.Lbrace,
					Message: "this nest runs serial in a function that already fans out elsewhere — the parallel transform is proven available; check the writes own disjoint output, then band the outer loop and gate with BOTH a bit-exact digest and -race",
				})
				return false // do not re-report inner loops of this nest
			})
		}
	}
	return nil, nil
}
