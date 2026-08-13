package checks

import (
	"fmt"
	"go/ast"
	"go/constant"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5009 reports math.Pow with a small constant exponent — an integer in
// 2..8 or exactly 0.5 — where a multiply chain or math.Sqrt is dramatically
// cheaper. Advisory only: the rewrite is unsafe to automate (see Doc).
var PS5009 = register(&lint.Check{
	ID:       "PS5009",
	Category: "arith",
	Slug:     "pow-constant-exponent",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "math.Pow with a small constant exponent",
		Text: `math.Pow is a general libm-style transcendental routine: even
for a compile-time constant exponent it pays the full log/exp machinery,
roughly 50-100x the cost of a couple of multiplications. A small integer
power is a multiply chain (x*x, x*x*x, ...) and math.Pow(x, 0.5) is
math.Sqrt(x) — a single hardware instruction on every modern CPU.

DELIBERATELY advisory — no automatic fix — for two reasons:

 1. The rewrite x*x evaluates the base expression TWICE, so it is unsafe
    when the base has side effects: math.Pow(f(), 2) calls f once, f()*f()
    calls it twice. Hoist the base into a local first.
 2. It is not bit-identical to math.Pow in all cases: for a NaN base,
    math.Pow(NaN, 2) returns the canonical NaN while x*x preserves the
    base's NaN payload — a math.Float64bits difference that fails
    perfscan's bit-identical bar. Likewise math.Sqrt diverges from
    math.Pow(x, 0.5) on a -Inf base (Pow returns +Inf, Sqrt returns NaN)
    and on a -0 base (Pow returns +0, Sqrt returns -0).

Only the low-noise exponents fire: exactly 0.5, or an integer in 2..8
inclusive. A variable exponent is exactly what math.Pow is for, exponents
0 and 1 are free already, and larger or fractional exponents have no
obviously better spelling — all of those stay silent.`,
		Before: `y := math.Pow(x, 2)
r := math.Pow(x, 0.5)`,
		After: `y := x * x        // only if x is side-effect-free; hoist f() first
r := math.Sqrt(x) // differs from Pow(x, 0.5) for -Inf and -0 bases`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5009",
		Doc:  "math.Pow with a small constant exponent",
		Run:  runPS5009,
	},
})

var mathPowSet = map[string]bool{"Pow": true}

func runPS5009(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "math", mathPowSet); !ok {
				return true
			}
			v := pass.TypesInfo.Types[call.Args[1]].Value
			if v == nil {
				return true // variable exponent: Pow is the right tool
			}
			if k := v.Kind(); k != constant.Int && k != constant.Float {
				return true
			}
			exp, exact := constant.Float64Val(v)
			if !exact {
				return true
			}
			var msg string
			switch {
			case exp == 0.5:
				msg = "math.Pow(x, 0.5) pays a general libm call where math.Sqrt(x) is far cheaper; not auto-fixed: math.Sqrt differs from math.Pow(x, 0.5) for -Inf and -0 bases"
			case exp == float64(int64(exp)) && exp >= 2 && exp <= 8:
				msg = fmt.Sprintf("math.Pow with constant exponent %d pays a general libm call ~50-100x slower than a multiply chain (x*x, x*x*x); not auto-fixed: x*x evaluates the base twice, and a NaN base yields a different NaN payload than math.Pow", int64(exp))
			default:
				return true // 0, 1, fractional, or large exponent: low-noise, silent
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: msg,
			})
			return true
		})
	}
	return nil, nil
}
