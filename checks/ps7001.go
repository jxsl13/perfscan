package checks

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS7001 scans embedded GPU compute kernels (Metal, initially) for a serial-K
// reduction: one thread per output row that loops the whole reduction dimension
// and accumulates alone, with no SIMD-group/subgroup cooperative reduction. At
// batch M=1 that leaves the other lanes of each SIMD group idle.
//
// Domain check: which embedded kernels are hot matvec/reduction kernels — and
// whether a serial reduction is the wrong choice for their shapes — is project
// vocabulary (config.gpuReductionKernels). It is both the opt-in and the scope:
// only the named kernels are inspected. With none listed the check stays silent.
var PS7001 = register(&lint.Check{
	ID:          "PS7001",
	Category:    "offload",
	Slug:        "gpu-serial-reduction",
	Level:       lint.LevelAggressive,
	NeedsConfig: true,
	Vocab:       []string{"gpuReductionKernels"},
	Doc: lint.Documentation{
		Title: "a serial-K GPU reduction kernel (one thread per row, no SIMD-group reduction) leaves lanes idle at M=1",
		Text: `A quantized GPU matvec kernel that assigns one thread to each
output row and serially accumulates the full reduction dimension K leaves the
device badly underutilized at batch M=1: every SIMD group runs a single active
lane while the rest idle. Splitting K across the SIMD-group lanes, processing a
couple of output rows per group, and reducing with a SIMD-group sum keeps all
lanes busy — a large, repeatable speedup on real hardware (see MeasuredWin).

PS7001 scans the Go STRING LITERALS that carry embedded Metal shader source and,
for each kernel whose entry-point name is listed in config.gpuReductionKernels,
flags the conjunction:
  - the kernel body has a per-thread for-loop (the K accumulation), AND
  - it contains NO SIMD-group / subgroup cooperative-reduction intrinsic
    (simd_sum, simd_shuffle*, quad_sum, simdgroup_*, …).

It is advisory only — never an unconditional rewrite. The cooperative version is
the right call only for large K with low output parallelism; it can LOSE when K
is small, the device already saturates, the kernel is memory-latency hidden,
deterministic accumulation order is required, subgroup support is unavailable, or
an autotuner already picks the serial path. Those are shape/dtype/device facts
the source cannot show, which is exactly why the check is opt-in per kernel and
recommends a measured, per-shape A/B benchmark rather than a fix.

Scope/limitations (v1): Metal only; the SIMD-reduction test is textual, so a
manual threadgroup-shared-memory reduction that uses no simd_* intrinsic is not
recognized as cooperative and could be reported — list only kernels you have
confirmed are serial. CUDA/Vulkan and non-string (separate .metal file) sources
are future work.`,
		Before: `kernel void matvec_q4k(...) {         // one thread per row, serial K
	uint row = tid.x;
	float acc = 0.0;
	for (uint k = 0; k < K; ++k) acc += w[row*K + k] * x[k];
	y[row] = acc;                          // no simd_sum: lanes idle at M=1
}`,
		After: `kernel void matvec_q4k(...) {         // K split across the SIMD group
	uint row = gid.y;
	float part = 0.0;
	for (uint k = lane; k < K; k += SIMD_W) part += w[row*K + k] * x[k];
	float acc = simd_sum(part);            // cooperative reduction
	if (lane == 0) y[row] = acc;
}`,
		MeasuredWin: `GoAI native Metal Q4_K on Apple M2 Pro (issue #565; resident
weights, 20 warmups, paired samples): K=2048,N=2048 ~546 us -> 250 us (2.18x);
K=5632,N=2048 ~1.20 ms -> 284 us (4.22x); K=2048,N=32000 ~973 us -> 595 us
(1.64x). Full TinyLlama-1.1B Q4_K_M decode 6.95 -> 9.9 tok/s (1.42x). Correctness
cross-checked against the scalar Metal and f64 GGUF reference; max relative error
within tolerance.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS7001",
		Doc:  "serial-K GPU reduction kernel with no SIMD-group cooperative reduction",
		Run:  runPS7001,
	},
})

// ps7001KernelDecl matches a Metal kernel entry point `kernel <quals> name(`,
// capturing the entry-point name (the identifier just before the parameter list).
var ps7001KernelDecl = regexp.MustCompile(`\bkernel\b[^;{}()]*?\b([A-Za-z_]\w*)\s*\(`)

// ps7001SIMDReduction matches any SIMD-group / subgroup cooperative-reduction
// intrinsic — its presence means the kernel already reduces cooperatively.
var ps7001SIMDReduction = regexp.MustCompile(`\b(simd_sum|simd_shuffle\w*|simd_broadcast\w*|simd_prefix\w*|simd_xor|simd_and|simd_or|simd_product|simd_max|simd_min|quad_sum|quad_shuffle\w*|quad_broadcast|quad_max|quad_min|simdgroup_\w+)\b`)

// ps7001ForLoop matches the per-thread accumulation loop.
var ps7001ForLoop = regexp.MustCompile(`\bfor\s*\(`)

func runPS7001(pass *analysis.Pass) (any, error) {
	kernels := config.Current().GPUReductionKernels
	if len(kernels) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			src, err := strconv.Unquote(lit.Value)
			if err != nil || !ps7001LooksMetal(src) {
				return true
			}
			for _, name := range ps7001SerialKernels(src, kernels) {
				pass.Report(analysis.Diagnostic{
					Pos:     lit.Pos(),
					End:     lit.End(),
					Message: "GPU kernel " + name + " accumulates the reduction dimension serially per thread with no SIMD-group cooperative reduction (no simd_sum/simd_shuffle); at batch M=1 one thread per output row leaves the other SIMD lanes idle — split K across the SIMD group and reduce with simd_sum, keeping the serial path where K is small or the device already saturates (benchmark per shape)",
				})
			}
			return true
		})
	}
	return nil, nil
}

// ps7001LooksMetal reports whether src is plausibly Metal shader source: it must
// declare a kernel and carry a Metal-specific marker, so an ordinary Go string
// that merely contains the word "kernel" is not scanned.
func ps7001LooksMetal(src string) bool {
	if !strings.Contains(src, "kernel") {
		return false
	}
	return strings.Contains(src, "thread_position_in_grid") ||
		strings.Contains(src, "metal_stdlib") ||
		strings.Contains(src, "[[buffer(") ||
		strings.Contains(src, "threadgroup_position_in_grid") ||
		strings.Contains(src, "thread_index_in_simdgroup")
}

// ps7001SerialKernels returns the names of the configured kernels in src whose
// body has a reduction for-loop but no SIMD-group cooperative reduction. Each
// kernel's region runs from its declaration to the next kernel declaration (or
// end of source), so a cooperative sibling in the same string is judged on its
// own body.
func ps7001SerialKernels(src string, want map[string]bool) []string {
	locs := ps7001KernelDecl.FindAllStringSubmatchIndex(src, -1)
	var out []string
	for i, m := range locs {
		name := src[m[2]:m[3]]
		if !want[name] {
			continue
		}
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		body := src[m[0]:end]
		if ps7001ForLoop.MatchString(body) && !ps7001SIMDReduction.MatchString(body) {
			out = append(out, name)
		}
	}
	return out
}
