package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6059 implements owner issue #719. It recognizes a compatible dual-
// projection/activation seam in typed recorder code and the corresponding
// larger-live-state shape in embedded or package-owned GPU source.
var PS6059 = register(&lint.Check{
	ID:       "PS6059",
	Category: "verify",
	Slug:     "gpu-fusion-is-resource-sensitive-hypothesis",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a GPU source-fusion opportunity has unpriced register and occupancy risk",
		Text: `Removing an intermediate activation and elementwise dispatch can
look mechanically profitable while making the fused GPU kernel slower. Keeping
two reductions live together may raise register pressure, reduce occupancy,
introduce a threadgroup exchange/barrier, duplicate weight traffic, or change
scheduling enough to outweigh the saved global traffic.

This check implements owner issue #719 through two direct source shapes:

  - typed Go recorder code with adjacent, same-receiver, compatible quantized
    projection/matmul calls whose single-consumer results feed one SwiGLU; and
  - embedded or package-owned Metal/CUDA/HIP/OpenCL/Vulkan source in which one
    GPU kernel updates distinct gate and up accumulator roles and consumes both
    in a SwiGLU/silu activation. An explicit barrier or shared/threadgroup state
    is named when present.

Different recorders, incompatible projection methods, reused intermediates,
ordinary CPU helpers, already-fused Go methods, separated kernels, commented
examples, and source without GPU markers stay silent. A finding describes the
possible saved intermediate traffic and dispatch, but labels it a hypothesis.
It never asserts a speedup from launch count, intermediate count, or one short
screen.

Before promoting a source fusion, benchmark the complete production seam under
identical command-buffer, transfer, workspace, dtype, shape, allocation,
quality, and odd-tail boundaries. Use repeated alternating paired samples with
a frozen gate. High confidence additionally needs device-and-shape repetition
or available shader register/occupancy/spill/stall counters; an unavailable
counter is unknown, not zero. There is NO automatic fix because the rewrite
changes numerical order, synchronization, resource lifetime, GPU source, and
device-specific occupancy.`,
		Before: `gate := rec.Q4KMatmul(x, gateWeight)
up := rec.Q4KMatmul(x, upWeight)
out := rec.SwiGLU(gate, up)`,
		After: `// Candidate only; do not promote from source shape or a short screen.
out := rec.ExperimentalQ4KPairedSwiGLU(x, gateWeight, upWeight)
// Retain only after counters/repeated paired complete-seam evidence passes.`,
		MeasuredWin: `On Apple M2 Pro, issue #719 schedule A appeared 1.196x
faster in a 200-iteration screen (517.365 us versus 432.731 us), but ten
alternating 500-iteration samples reversed it: 396.1 us control versus 406.3 us
candidate median (p=0.436, n=10), a 2.6% regression with high variability.
Schedule B reduced live state through separate SIMD groups and a threadgroup
exchange but was already 3.4% slower (420.880 us versus 435.214 us). Both kept
8 B/op and 1 alloc/op, failed the frozen 1.10x leaf gate, and were removed.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6059",
		Doc:  "GPU fusion opportunity has unpriced register, synchronization, and occupancy risk",
		Run:  runPS6059,
	},
})

var (
	ps6059AccumulatorDeclaration = regexp.MustCompile(`\b(?:half|float|double|bfloat|int|uint|short|ushort|long|ulong|int32_t|uint32_t)(?:[2-4])?\s+([A-Za-z_]\w*)\s*(?:\[[^\]\n]+\])?\s*(?:=|;)`)
	ps6059Barrier                = regexp.MustCompile(`\b(?:threadgroup_barrier|simdgroup_barrier|__syncthreads|barrier|work_group_barrier|GroupMemoryBarrierWithGroupSync)\s*\(`)
	ps6059SharedState            = regexp.MustCompile(`\b(?:threadgroup|__shared__|groupshared|__local|shared)\s+(?:half|float|double|bfloat|int|uint|short|ushort|long|ulong|int32_t|uint32_t)\b`)
)

type ps6059NativeFinding struct {
	kernel  string
	gate    string
	up      string
	barrier bool
	shared  bool
	offset  int
}

func runPS6059(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Body != nil {
				ps6059ReportGoSeams(pass, fn)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			source, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, finding := range ps6059NativeFindings(source) {
				pass.Report(analysis.Diagnostic{Pos: lit.Pos(), End: lit.End(), Message: ps6059NativeMessage(finding)})
			}
			return true
		})
	}

	seen := make(map[string]bool, len(pass.OtherFiles)+len(pass.IgnoredFiles))
	for _, filename := range slices.Concat(pass.OtherFiles, pass.IgnoredFiles) {
		if seen[filename] || !ps6053NativeExtension(filename) {
			continue
		}
		seen[filename] = true
		source, err := ps6053ReadFile(pass, filename)
		if err != nil {
			continue
		}
		findings := ps6059NativeFindings(string(source))
		if len(findings) == 0 {
			continue
		}
		file := pass.Fset.AddFile(filename, -1, len(source))
		file.SetLinesForContent(source)
		for _, finding := range findings {
			offset := min(max(finding.offset, 0), len(source))
			pass.Reportf(file.Pos(offset), "%s", ps6059NativeMessage(finding))
		}
	}
	return nil, nil
}

func ps6059ReportGoSeams(pass *analysis.Pass, fn *ast.FuncDecl) {
	ps6032Blocks(fn.Body, func(block *ast.BlockStmt) {
		for index := 0; index+2 < len(block.List); index++ {
			gate, gateOK := ps6057ProducerStmt(pass, block.List[index])
			up, upOK := ps6057ProducerStmt(pass, block.List[index+1])
			if !gateOK || !upOK || gate.name != up.name || gate.recv == "" || gate.recv != up.recv || !ps6022CommandType(gate.typeOf) || !ps6022CommandType(up.typeOf) {
				continue
			}
			if !ps6032SingleUse(pass, fn.Body, gate.result) || !ps6032SingleUse(pass, fn.Body, up.result) {
				continue
			}
			consumer, ok := ps6057ConsumerStmt(pass, block.List[index+2], []types.Object{gate.result, up.result})
			if !ok || consumer.recv != gate.recv || !ps6057PairedConsumer(consumer.name) {
				continue
			}
			pass.Reportf(gate.call.Pos(), "adjacent compatible %s/%s -> %s GPU calls expose a possible saved intermediate and dispatch, but fusion is only a hypothesis: price added live accumulators/register pressure, spills, synchronization, duplicated reads, and occupancy; require identical complete-seam boundaries, allocations and quality plus repeated alternating device-and-shape evidence or available shader register/occupancy/stall counters before promotion (advisory, no automatic fix)", gate.name, up.name, consumer.name)
			index += 2
		}
	})
}

func ps6059NativeFindings(source string) []ps6059NativeFinding {
	clean := ps6053BlankCommentsAndStrings(source)
	if !ps6053LooksGPU(clean) {
		return nil
	}
	var findings []ps6059NativeFinding
	for _, kernel := range ps6053Kernels(clean) {
		body := clean[kernel.start:kernel.end]
		gate, up := "", ""
		for _, match := range ps6059AccumulatorDeclaration.FindAllStringSubmatchIndex(body, -1) {
			name := body[match[2]:match[3]]
			if !ps6059AccumulatorUpdated(body, name) {
				continue
			}
			switch ps6059AccumulatorRole(name) {
			case "gate":
				if gate == "" {
					gate = name
				}
			case "up":
				if up == "" {
					up = name
				}
			}
		}
		if gate == "" || up == "" || !ps6059ActivationUses(body, gate, up) {
			continue
		}
		findings = append(findings, ps6059NativeFinding{
			kernel:  kernel.name,
			gate:    gate,
			up:      up,
			barrier: ps6059Barrier.MatchString(body),
			shared:  ps6059SharedState.MatchString(body),
			offset:  kernel.start,
		})
	}
	return findings
}

func ps6059AccumulatorRole(name string) string {
	name = ps6007NormalizeName(name)
	switch {
	case strings.Contains(name, "gate") && ps6007ContainsAny(name, "acc", "sum", "partial", "reduced"):
		return "gate"
	case strings.Contains(name, "up") && ps6007ContainsAny(name, "acc", "sum", "partial", "reduced"):
		return "up"
	default:
		return ""
	}
}

func ps6059AccumulatorUpdated(body, name string) bool {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*(?:\[[^\]]+\])?\s*(?:\+=|-=|\*=|/=)`)
	return pattern.MatchString(body)
}

func ps6059ActivationUses(body, gate, up string) bool {
	gate = regexp.QuoteMeta(gate)
	up = regexp.QuoteMeta(up)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)\bswiglu\s*\([^;{}]*\b` + gate + `\b[^;{}]*\b` + up + `\b[^;{}]*\)`),
		regexp.MustCompile(`(?is)\bsilu\s*\([^;{}]*\b` + gate + `\b[^;{}]*\)\s*\*[^;{}]*\b` + up + `\b`),
		regexp.MustCompile(`(?is)\b` + up + `\b[^;{}]*\*\s*(?:precise\s*::\s*)?silu\s*\([^;{}]*\b` + gate + `\b[^;{}]*\)`),
	}
	return slices.ContainsFunc(patterns, func(pattern *regexp.Regexp) bool { return pattern.MatchString(body) })
}

func ps6059NativeMessage(finding ps6059NativeFinding) string {
	risk := "two simultaneously live accumulator roles increase register pressure and can lower occupancy"
	if finding.barrier || finding.shared {
		risk += "; shared/threadgroup exchange or barriers add synchronization and scheduling risk"
	}
	return "GPU kernel " + finding.kernel + " combines gate/up reductions and activation through " + finding.gate + "/" + finding.up + "; removing an intermediate and dispatch is only a hypothesis because " + risk + " — benchmark identical complete production seams with allocation/quality gates and repeated alternating device-and-shape samples; require available register/occupancy/spill/stall counters for high confidence, treating unavailable counters as unknown (advisory, no automatic fix)"
}
