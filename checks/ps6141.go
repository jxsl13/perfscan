package checks

import (
	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
)

var PS6141 = register(&lint.Check{
	ID: "PS6141", Category: "verify", Slug: "unamortized-activation-quantization", Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true, Vocab: []string{"singleUseQuantizationContracts"},
	Doc: lint.Documentation{
		Title: "fresh activation packing immediately feeds one dot boundary without visible amortization",
		Text: `PS6141 is an opt-in verification advisory. Configure exact typed quantizer and consumer identities and review their quantization/dot meaning. Source analysis separately proves fresh storage, effects, normal completion, and one immediate returned dot reduction. Multi-function allocation/fill/consumer wrappers and current-block fixed lanes are supported within conservative typed source subsets. The packedByteDot form additionally proves returned signed lane/header origins and exact producer stride. Unknown helper/native effects, aliases, escapes, scratch/cache reuse, multiple consumers or output rows, and unsupported dynamic traversal stay silent; M=1 alone never proves one dot when N rows reuse the activation.

The advisory does not recommend reciprocal or approximate arithmetic, establish numerical equivalence, or promise a speedup. Benchmark the complete conversion-and-consumer boundary at the relevant shape before changing it. A strict reviewed-policy exemption binds exact source/site/shape and observed loader target; it is applicability policy, not independently reproduced benchmark evidence or CPU identification. There is no automatic fix. Authentic retained kernels consume float activation with packed weights or use opaque native/table effects; mixed test scaffolds are not historical activation-positive provenance.

No portable isolated before/after benchmark can validate a universal transform: this check identifies an amortization risk, and its remedy depends on the actual whole boundary, precision contract, target, shape and reuse lifetime. Preserve numerical and allocation/lifecycle validation alongside whole-boundary measurements.`,
		Before: `p := quantize(activation)
return dot(p, weights)`,
		After: `// Benchmark complete conversion + consumer at this shape and target.
// Preserve precision and lifetime contracts; reuse, fuse, or avoid packing only
// when the measured whole-boundary result justifies that project-specific change.`,
		MeasuredWin: `Owner issue #835 reports Apple M2 Pro Q4_K x Q8_K activation packing plus an exact oracle-validated I8MM kernel at M=1,N=4096,K=1024: median established F32-activation path 182032 ns/op versus packed candidate 208481 ns/op, approximately 0.87x with one extra allocation. A single-row SDOT variant peaked near 1.18x, below the 1.5x leaf gate. These are attributed owner risk evidence, not locally reproduced measurements, a universal numeric-equivalence claim, or a positive detector fixture. Source fixtures are synthetic/MIXED; actual row reuse and fused/native boundaries remain negative/unknown. https://github.com/jxsl13/perfscan/issues/835`,
	}, Analyzer: &analysis.Analyzer{Name: "PS6141", Doc: "source-proved single-use fresh activation packing amortization risk", Run: runPS6141},
})

func runPS6141(pass *analysis.Pass) (any, error) {
	return runPS6141WithVocabulary(pass, config.Current)
}

func runPS6141WithVocabulary(pass *analysis.Pass, current func() config.Sets) (any, error) {
	return runPS6141WithContracts(pass, current().SingleUseQuantizationContracts)
}
