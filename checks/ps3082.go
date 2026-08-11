package checks

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3082 reports math.Min/math.Max calls inside loops (excluding clamps,
// which PS3077 owns).
var PS3082 = register(&lint.Check{
	ID:       "PS3082",
	Category: "indirect",
	Slug:     "minmax-call-in-loop",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "math.Min or math.Max called inside a loop",
		Text: `On arm64, math.Min compiles to CALL math.archMin inside a
48-byte frame with a stack-growth check; the min/max builtins compile to a
single instruction in a leaf frame. But the two are NOT the same function:
math.Max documents +Inf as beating NaN and math.Min documents -Inf as
beating NaN, while the builtins propagate NaN — they disagree on exactly
four ordered pairs. Substituting raw builtins is a real bug on NaN-carrying
data.

The remedy is a thin wrapper that takes the instruction and consults math
only when the instruction returns NaN (the only case they can disagree on).
It does NOT beat an existing comparison chain — this replaces calls, not
branchless code. Gate the change on one planted NaN/Inf value per call
site; a kernel that reduces to a scalar cannot see the divergence.`,
		Before: `for _, v := range xs {
	hi = math.Max(hi, v)
}`,
		After: `// fmax: the max builtin, falling back to math.Max only on NaN.
func fmax(a, b float64) float64 {
	if r := max(a, b); r == r { return r }
	return math.Max(a, b)
}

for _, v := range xs {
	hi = fmax(hi, v)
}`,
		MeasuredWin: "goai reference: PPOClip batch 4096 -58.7% (100.5µs→41.6µs), GRPO -37.0%; converting an existing comparison chain measured +13% and was reverted",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3082",
		Doc:  "math.Min/math.Max call in a loop",
		Run:  runPS3082,
	},
})

func runPS3082(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			name, call, ok := isMathMinMax(expr)
			if !ok {
				return true
			}
			// Clamps belong to PS3077: skip a call that wraps the
			// opposite call, and skip a call directly wrapped by one.
			for _, arg := range call.Args {
				if inner, _, ok := isMathMinMax(arg); ok && inner != name {
					return true
				}
			}
			if len(stack) > 0 {
				if outerName, _, ok := isMathMinMax(exprOrNil(stack[len(stack)-1])); ok && outerName != name {
					return true
				}
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "math." + name + " in a loop pays a function call per iteration; the " + map[string]string{"Min": "min", "Max": "max"}[name] + " builtin is one instruction but differs on NaN-vs-Inf pairs — use a NaN-correct wrapper and gate with planted edge values",
			})
			return true
		})
	}
	return nil, nil
}

func exprOrNil(n ast.Node) ast.Expr {
	e, _ := n.(ast.Expr)
	return e
}
