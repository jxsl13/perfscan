package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS4002 reports scalar transcendental calls in loops of functions that
// already call a configured hand-vectorized SIMD kernel for another dtype.
var PS4002 = register(&lint.Check{
	ID:          "PS4002",
	Category:    "vector",
	Slug:        "scalar-transcendental-vectorizable",
	Level:       lint.LevelAggressive,
	NeedsConfig: true,
	Vocab:       []string{"vectorizedSiblingFuncs"},
	Doc: lint.Documentation{
		Title: "a scalar math transcendental in a loop beside a vectorized sibling kernel",
		Text: `When one dtype branch of a kernel runs math.Exp/Tanh/… scalar in
a loop while the same function calls a hand-vectorized sibling (vexpF32,
vsiluF32, …) for another dtype, the scalar branch is a proven candidate
for the same SIMD treatment — the vectorized sibling is the evidence that
the transform is available and worth it here.

L3 (aggressive): a SIMD transcendental is not bit-identical to the libm
scalar; gate the new kernel exactly as the existing sibling was gated
(tolerance oracle or documented ulp bound).`,
		Before: `case F32:
	vsiluF32(dst, src)          // hand-vectorized
case F64:
	for i := range src {
		dst[i] = src[i] / (1 + math.Exp(-src[i])) // scalar, per element
	}`,
		After: `case F32:
	vsiluF32(dst, src)
case F64:
	vsiluF64(dst, src) // same SIMD treatment, tolerance-gated like F32`,
		MeasuredWin: "reference corpus: an AVX2 f64 exp beside the existing f32 kernel measured 1.52x on end-to-end prefill",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS4002",
		Doc:  "scalar transcendental beside a vectorized sibling",
		Run:  runPS4002,
	},
})

var scalarTranscendentals = map[string]bool{
	"Exp": true, "Expm1": true, "Log": true, "Log1p": true, "Log2": true, "Log10": true,
	"Tanh": true, "Sinh": true, "Cosh": true, "Erf": true, "Erfc": true, "Pow": true,
}

func runPS4002(pass *analysis.Pass) (any, error) {
	vectorized := config.Current().VectorizedSiblingFuncs
	if len(vectorized) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !callsFanOut(fn.Body, vectorized) {
				continue
			}
			reportedLoop := map[ast.Node]bool{}
			astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "math", scalarTranscendentals)
				if !ok {
					return true
				}
				loop, inLoop := astutil.InLoop(stack)
				if !inLoop || reportedLoop[loop] {
					return true
				}
				reportedLoop[loop] = true
				pass.Report(analysis.Diagnostic{
					Pos:     call.Pos(),
					End:     call.End(),
					Message: fmt.Sprintf("scalar math.%s per element in a function that already calls a vectorized sibling kernel — this branch is a proven SIMD candidate; gate the new kernel like the sibling was gated", name),
				})
				return true
			})
		}
	}
	return nil, nil
}
