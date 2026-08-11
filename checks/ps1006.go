package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS1006 reports a reduction whose INNER loop variable is the high-stride
// (multiplied) part of a flat index while the outer variable is the
// contiguous (additive) part.
var PS1006 = register(&lint.Check{
	ID:       "PS1006",
	Category: "access",
	Slug:     "strided-inner-reduction",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "a reduction striding a flat array by the inner loop variable",
		Text: `ARR[inner*stride + outer] strides ARR by stride every inner
step — cache-thrashing when stride × working set exceeds L2. Interchange
the loops so ARR is walked contiguously; per output element the reduction
keeps its order, so the result is bit-identical.

The win SCALES WITH stride × working-set: big when it exceeds L2, ~noise
when L1-resident — rank candidates by the strided dimension's size. When
the strided access sits inside a fused scan that cannot be interchanged,
the remedy is GATHER/SCATTER: copy the column into contiguous scratch
once, scan the scratch, scatter back once — same bit-exact remedy.`,
		Before: `for c := 0; c < cols; c++ {
	s := 0.0
	for r := 0; r < rows; r++ {
		s += a[r*cols+c] // strides by cols every step
	}
	out[c] = s
}`,
		After: `sums := make([]float64, cols)
for r := 0; r < rows; r++ { // contiguous walk
	base := r * cols
	for c := 0; c < cols; c++ {
		sums[c] += a[base+c]
	}
}
copy(out, sums)`,
		MeasuredWin: "goai reference: MLA value-mix 1.13x/1.27x, spectral-norm power-iter 2.57x; WKV backward 2.2–2.9x via the gather/scatter form",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1006",
		Doc:  "inner loop strides a flat array",
		Run:  runPS1006,
	},
})

func runPS1006(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			iv, body := loopVarAndBody(n)
			if iv == "" || body == nil || containsLoop(body) {
				return true
			}
			outerLoop, in := astutil.InLoop(stack)
			if !in {
				return true
			}
			ov := outermostLoopVar(outerLoop)
			if ov == "" || ov == iv {
				return true
			}
			// A read ARR[iv*s + ov-part]: the inner variable multiplied,
			// the outer variable additive.
			var hit *ast.IndexExpr
			ast.Inspect(body, func(m ast.Node) bool {
				if hit != nil {
					return false
				}
				ix, ok := m.(*ast.IndexExpr)
				if !ok {
					return true
				}
				if stridedByInner(ix.Index, iv, ov) {
					hit = ix
					return false
				}
				return true
			})
			if hit == nil {
				return true
			}
			// Must be a reduction: an accumulation into a target free of
			// the inner variable.
			reduces := false
			ast.Inspect(body, func(m ast.Node) bool {
				as, ok := m.(*ast.AssignStmt)
				if !ok || as.Tok != token.ADD_ASSIGN && as.Tok != token.SUB_ASSIGN {
					return true
				}
				for _, lhs := range as.Lhs {
					if !exprMentions(lhs, iv) {
						reduces = true
						return false
					}
				}
				return true
			})
			if !reduces {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: "the inner loop variable is the multiplied (high-stride) part of this flat index — the array is strided every inner step; interchange the loops so it is walked contiguously (bit-identical per output element), or gather into contiguous scratch when the scan cannot be interchanged",
			})
			return true
		})
	}
	return nil, nil
}

// stridedByInner matches iv*s + <term with ov> / <term> + iv*s: the inner
// variable inside a product, an additive sibling mentioning the outer
// variable but not the inner one.
func stridedByInner(e ast.Expr, iv, ov string) bool {
	be, ok := e.(*ast.BinaryExpr)
	if !ok || be.Op != token.ADD {
		return false
	}
	isMulWithIv := func(x ast.Expr) bool {
		m, ok := x.(*ast.BinaryExpr)
		return ok && m.Op == token.MUL && exprMentions(m, iv)
	}
	additiveOK := func(x ast.Expr) bool {
		return exprMentions(x, ov) && !exprMentions(x, iv)
	}
	return (isMulWithIv(be.X) && additiveOK(be.Y)) || (isMulWithIv(be.Y) && additiveOK(be.X))
}
