package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS1010 reports a nested loop over a slice of slices whose inner loop
// varies the ROW index — a column walk that jumps a whole row per step.
var PS1010 = register(&lint.Check{
	ID:       "PS1010",
	Category: "access",
	Slug:     "column-walk-slice-of-slices",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "an inner loop reading one column of a [][]T (a row-jumping walk)",
		Text: `for j { for i { X[i][j] } } re-dereferences a row header and
touches a fresh cache line to read eight bytes, every step. The flat-array
strided checks cannot see this: they match index arithmetic, and X[i][j]
has none — a blind spot the reference paid for twice before adding this
check.

Reported ONLY when interchange is profitable: the inner loop must assign to
something that does not mention the inner variable — a scalar accumulator,
or a slot indexed only by the outer one. A transpose (out[j][i] = in[i][j])
strides whichever way it runs and is excluded by that clause. An inner body
that itself contains a loop is skipped (amortization filter): the strided
access then happens once against that loop's whole trip count.

Bit-identity is usually free — interchanging preserves each accumulator's
summation order when the accumulation is per-outer-index — but CONFIRM per
site. The check finds the SHAPE, not the payoff: the reference measured
-23.7% on one site and +3.7% (reverted) on another, so benchmark at the
site.`,
		Before: `for j := 0; j < d; j++ {
	s := 0.0
	for i := range rows {
		s += rows[i][j] // jumps a whole row per step
	}
	mean[j] = s / float64(len(rows))
}`,
		After: `sums := make([]float64, d)
for i := range rows { // row-major: one contiguous pass
	for j, v := range rows[i] {
		sums[j] += v
	}
}
for j := range mean {
	mean[j] = sums[j] / float64(len(rows))
}`,
		MeasuredWin: "goai reference: GaussianNB.Fit -23.74% folding 2d column walks into two row-major passes; ballTree.build -18.71% on KNNFit; CholSolve's l[k][i] measured +3.74% and was reverted — confirm at the site",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1010",
		Doc:  "column walk over a slice of slices",
		Run:  runPS1010,
	},
})

func isSliceOfSlices(t types.Type) bool {
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	_, ok = s.Elem().Underlying().(*types.Slice)
	return ok
}

func runPS1010(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			iv, body := loopVarAndBody(n)
			if iv == "" || body == nil {
				return true
			}
			// Must sit inside an enclosing loop (the column index varies
			// somewhere above) and have no nested loop (amortization
			// filter).
			if _, in := astutil.InLoop(stack); !in {
				return true
			}
			if containsLoop(body) {
				return true
			}
			// Interchange must be profitable: every assignment target in
			// the body must not mention the inner variable.
			profitable := true
			ast.Inspect(body, func(m ast.Node) bool {
				as, ok := m.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range as.Lhs {
					if exprMentions(lhs, iv) {
						profitable = false
						return false
					}
				}
				return true
			})
			if !profitable {
				return true
			}
			// A column read: X[iv][c] with c free of iv and X a [][]T.
			var hit *ast.IndexExpr
			var base string
			ast.Inspect(body, func(m ast.Node) bool {
				if hit != nil {
					return false
				}
				outer, ok := m.(*ast.IndexExpr)
				if !ok {
					return true
				}
				row, ok := outer.X.(*ast.IndexExpr)
				if !ok {
					return true
				}
				if !exprMentions(row.Index, iv) || exprMentions(outer.Index, iv) {
					return true
				}
				t := pass.TypesInfo.TypeOf(row.X)
				if t == nil || !isSliceOfSlices(t) {
					return true
				}
				hit = outer
				base = baseIdentName(row.X)
				return false
			})
			if hit == nil {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: fmt.Sprintf("inner loop walks a COLUMN of %s — a row header dereference and a fresh cache line per 8-byte read; interchange to a row-major pass (accumulators are per-outer-index here, so summation order per output is preserved — confirm per site)", base),
			})
			return true
		})
	}
	return nil, nil
}
