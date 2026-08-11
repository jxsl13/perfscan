package checks

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6010 reports an output loop whose inner accumulator re-reads an
// operand that does not vary with the output index.
var PS6010 = register(&lint.Check{
	ID:       "PS6010",
	Category: "verify",
	Slug:     "output-invariant-operand-reload",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "an accumulator loop re-reading an operand invariant in the output index",
		Text: `for o { for i { acc += A[i] * B[f(i,o)] } } re-streams all of A
once per output element: A[i] does not vary with o, so unrolling the OUTPUT
loop by 4 — four accumulators sharing one pass over A — amortizes that load
across four outputs (register blocking / unroll-and-jam).

L3 (aggressive): each accumulator keeps its own order, so the per-output
sums are bit-identical at any unroll factor; the tail loop handles the
remainder. This is the inline-accumulator sibling of the restreamed-row
shape (PS1007) — here the INPUT is re-read, there the OUTPUT is
re-streamed. Rank by the loop's share of runtime × the achievable factor.`,
		Before: `for o := 0; o < out; o++ {
	acc := 0.0
	for i := 0; i < n; i++ {
		acc += a[i] * w[i*out+o] // a re-streamed per output
	}
	dst[o] = acc
}`,
		After: `for o := 0; o+3 < out; o += 4 { // 4 accumulators share one pass over a
	var a0, a1, a2, a3 float64
	for i := 0; i < n; i++ {
		ai := a[i]
		a0 += ai * w[i*out+o]
		a1 += ai * w[i*out+o+1]
		a2 += ai * w[i*out+o+2]
		a3 += ai * w[i*out+o+3]
	}
	dst[o], dst[o+1], dst[o+2], dst[o+3] = a0, a1, a2, a3
}
// (serial tail for the remainder)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6010",
		Doc:  "operand re-read per output element in an accumulator loop",
		Run:  runPS6010,
	},
})

func runPS6010(pass *analysis.Pass) (any, error) {
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
			var hit ast.Expr
			ast.Inspect(body, func(m ast.Node) bool {
				if hit != nil {
					return false
				}
				as, ok := m.(*ast.AssignStmt)
				if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					return true
				}
				// Accumulator must be a scalar free of both loop vars.
				if _, ok := as.Lhs[0].(*ast.Ident); !ok {
					return true
				}
				mul, ok := as.Rhs[0].(*ast.BinaryExpr)
				if !ok || mul.Op != token.MUL {
					return true
				}
				// One factor indexed by the inner var only (invariant in
				// the output index), the other mentioning the outer var.
				check := func(x, y ast.Expr) bool {
					xi, ok := x.(*ast.IndexExpr)
					if !ok {
						return false
					}
					return exprMentions(xi.Index, iv) && !exprMentions(xi.Index, ov) && exprMentions(y, ov)
				}
				if check(mul.X, mul.Y) {
					hit = mul.X
				} else if check(mul.Y, mul.X) {
					hit = mul.Y
				}
				return hit == nil
			})
			if hit == nil {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: "this operand does not vary with the output index " + ov + " but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load (bit-identical per output)",
			})
			return true
		})
	}
	return nil, nil
}
