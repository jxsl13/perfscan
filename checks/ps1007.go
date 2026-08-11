package checks

import (
	"fmt"
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS1007 reports an inner loop accumulating into a loop-INVARIANT output
// row — all of OUT is loaded and stored once per OUTER step instead of
// once in total.
var PS1007 = register(&lint.Check{
	ID:       "PS1007",
	Category: "access",
	Slug:     "output-row-restreamed",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "an inner loop accumulating into an output row that does not vary with the outer loop",
		Text: `OUT[d] += f(outer)*IN[…] re-streams all of OUT once per outer
step: per element that is three memory ops (load IN, load OUT, store OUT)
times the outer trip count.

TWO remedies, and WHICH one depends on whether IN is already contiguous in
the inner variable — both bit-identical when OUT enters zeroed, since each
element still sums the outer variable ascending:

(a) IN is NOT contiguous in d (a d-strided gather): strip-mine the d loop
by 4 and hold four partial sums in registers with the outer loop innermost.

(b) IN IS contiguous in d (a row-major rank-1 update): do NOT strip-mine —
that trades one contiguous pass for strided ones and its gain decays with
the outer trip count. Instead unroll the OUTER loop by 2 with separate
accumulating adds, so OUT[d] stays in a register across the pair.

(c) SEVERAL output rows already accumulated together (a band kernel):
(b)'s refusal does not transfer — strip-mining there is a register TILE
and the removed output traffic dominates. The separating quantity is
arithmetic intensity: one output row gives 4 FMAs per 5 loads, four rows
give 16 per 8.

L3 (aggressive): rank by the loop's share of runtime × the achievable
unroll factor, not by cache residency — the saving is a fixed fraction of
the loop's own traffic either way.`,
		Before: `for i := 0; i < n; i++ {
	v := w[i]
	for d := 0; d < dim; d++ {
		out[d] += v * in[i*dim+d] // out fully re-streamed per i
	}
}`,
		After: `for i := 0; i+1 < n; i += 2 { // case (b): outer unroll, separate adds
	v0, v1 := w[i], w[i+1]
	r0, r1 := in[i*dim:(i+1)*dim], in[(i+1)*dim:(i+2)*dim]
	for d := 0; d < dim; d++ {
		out[d] += v0 * r0[d]
		out[d] += v1 * r1[d] // out[d] stays in a register across the pair
	}
}
// (serial tail for odd n)`,
		MeasuredWin: "reference corpus: case (a) sparse P·V 1.24x; case (b) QR outer-unroll geomean -2.69%, SnapKV -21.08% at U=4; case (c) band kernels -31..-50% with the gain growing with size",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1007",
		Doc:  "inner loop accumulates into an outer-invariant output row",
		Run:  runPS1007,
	},
})

func runPS1007(pass *analysis.Pass) (any, error) {
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
			var hit *ast.IndexExpr
			ast.Inspect(body, func(m ast.Node) bool {
				if hit != nil {
					return false
				}
				as, ok := m.(*ast.AssignStmt)
				if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					return true
				}
				ix, ok := as.Lhs[0].(*ast.IndexExpr)
				if !ok {
					return true
				}
				// The output slot varies with the inner variable only —
				// the row is outer-invariant.
				if exprMentions(ix, ov) || !exprMentions(ix.Index, iv) {
					return true
				}
				// The added term must involve the outer iteration (a
				// weight or a row read varying with it) — otherwise the
				// accumulation is a plain per-element sum.
				if !exprMentions(as.Rhs[0], ov) && !rhsMentionsOuterLocal(as.Rhs[0], outerLoop, body) {
					return true
				}
				hit = ix
				return false
			})
			if hit == nil {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: fmt.Sprintf("this inner loop accumulates into an output row invariant in outer variable %s — all of it is loaded and stored once per outer step; the remedy depends on whether the input is contiguous in %s (strip-mine the gather, outer-unroll the rank-1 update, register-tile a band) — see PS1007 docs", ov, iv),
			})
			return true
		})
	}
	return nil, nil
}

// rhsMentionsOuterLocal reports whether the accumulated term mentions a
// local bound in the OUTER loop body before the inner loop (v := w[i]
// hoisted by hand) — the shape still varies per outer step.
func rhsMentionsOuterLocal(rhs ast.Expr, outerLoop ast.Node, innerBody *ast.BlockStmt) bool {
	outerBody := astutil.LoopBody(outerLoop)
	if outerBody == nil {
		return false
	}
	locals := map[string]bool{}
	ast.Inspect(outerBody, func(n ast.Node) bool {
		if n == ast.Node(innerBody) {
			return false // stop at the inner loop
		}
		if as, ok := n.(*ast.AssignStmt); ok && as.Tok == token.DEFINE {
			for _, lhs := range as.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
					locals[id.Name] = true
				}
			}
		}
		return true
	})
	return mentionsAnyOf(rhs, locals)
}
