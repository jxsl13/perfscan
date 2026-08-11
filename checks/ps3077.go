package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3077 reports a two-bound clamp written as math.Min(math.Max(...)) (or
// the reverse) inside a loop.
var PS3077 = register(&lint.Check{
	ID:       "PS3077",
	Category: "indirect",
	Slug:     "minmax-clamp-in-loop",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "math.Min wrapped around math.Max (a clamp) inside a loop",
		Text: `A clamp written as two math calls carries the whole NaN and
signed-zero contract every iteration. A comparison chain (or the min/max
builtins via a NaN-correct wrapper) is dramatically cheaper — but the naive
rewrite is a real bug: use 'if r <= lo' rather than '<' (math.Max(0,-0)
returns +0 where '<' lets -0 through), and NaN must fall through both
bounds untouched.

L3 (aggressive): the rewrite must be gated twice — a bit-for-bit table over
-0, +0, NaN, both infinities and each boundary, AND a digest of the caller,
since ordinary data never produces the cases that make the naive rewrite
wrong.`,
		Before: `for i, v := range xs {
	xs[i] = math.Min(math.Max(v, lo), hi)
}`,
		After: `for i, v := range xs {
	r := v
	if r <= lo { r = lo } // <=, not <: preserves math.Max's -0 handling
	if r >= hi { r = hi }
	// NaN falls through both bounds untouched, matching math.Min/Max order
	xs[i] = r
}`,
		MeasuredWin: "goai reference: HQQ quantizer -51.0% (77.14ms→37.79ms, 2.04x) with archMin/archMax leaving the profile",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3077",
		Doc:  "math.Min/math.Max clamp in a loop",
		Run:  runPS3077,
	},
})

var mathMinMax = map[string]bool{"Min": true, "Max": true}

// isMathMinMax reports whether e is a math.Min or math.Max call.
func isMathMinMax(e ast.Expr) (string, *ast.CallExpr, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return "", nil, false
	}
	name, ok := astutil.PkgFuncCall(call.Fun, "math", mathMinMax)
	return name, call, ok
}

func runPS3077(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			outer, outerCall, ok := isMathMinMax(expr)
			if !ok {
				return true
			}
			inner := ""
			for _, arg := range outerCall.Args {
				if name, _, ok := isMathMinMax(arg); ok && name != outer {
					inner = name
					break
				}
			}
			if inner == "" {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     outerCall.Pos(),
				End:     outerCall.End(),
				Message: "a clamp written as math." + outer + "(math." + inner + "(…)) pays two calls with the full NaN/signed-zero contract per iteration; a comparison chain is far cheaper but must be gated on -0/NaN/Inf edge cases",
			})
			return true
		})
	}
	return nil, nil
}
