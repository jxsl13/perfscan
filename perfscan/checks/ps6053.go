package checks

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6053 implements owner issue #725. It recognizes the narrow source shape
// where every GPU lane owns a full logical-vector accumulator and a subgroup
// reduction subsequently shuffles every element of that accumulator.
var PS6053 = register(&lint.Check{
	ID:       "PS6053",
	Category: "verify",
	Slug:     "gpu-full-vector-accumulator-replicated-per-lane",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a full logical-vector accumulator is replicated and shuffled in every GPU lane",
		Text: `A decode-attention or reduction kernel can assign the small
reduction axis to SIMD lanes while keeping a full output-vector array private
in every lane. Every lane then pays the array's register footprint, and the
final tree reduction shuffles every array element at every level. Occupancy and
shuffle traffic can collapse when only one or two lanes have useful work.

This check implements owner issue #725. It scans embedded Go string literals
and package-owned native GPU sources (including go:embed .metal, .cu, .cuh,
.hip, .comp, .glsl, and OpenCL/C-family files). It reports only the conjunction:

  - the source has Metal, CUDA/HIP, Vulkan/GLSL, or OpenCL GPU markers;
  - a kernel maps a subgroup/workgroup to a logical row or head;
  - a lane-private fixed array is accumulator-like and its bound is at least
    32 or names a logical vector/head dimension;
  - an unstriped zero-to-bound loop updates every array element per lane; and
  - another zero-to-bound loop applies a subgroup shuffle to every element.

Threadgroup/shared arrays, small unrelated arrays, kernels whose lanes already
own disjoint dimensions, and arrays not participating in the shuffle reduction
are suppressed. C/C++ comments and quoted strings are blanked before matching,
so examples and disabled code do not create findings.

The advisory recommends evaluating the dual decomposition: stream or broadcast
the smaller reduction axis while lanes own disjoint vector dimensions. It must
remain behind a measured selector. Require a shape crossover, unchanged
allocation counts, a numerical-order/parity gate, retained-shape controls, and
an end-to-end benchmark. There is NO automatic fix because the profitable
crossover and acceptable floating-point order are device, dtype, and shape
facts, and because an unconditional replacement loses when the streamed axis
grows.`,
		Before: `float acc[128] = {0};                 // private in every lane
for (uint d = 0; d < 128; ++d) acc[d] += score * value[d];
for (uint d = 0; d < 128; ++d)
    acc[d] += simd_shuffle_down(acc[d], offset);`,
		After: `// selector: sq == 1 && sk <= measuredCrossover
float score = lane == 0 ? serial_dot(q, k) : 0;
score = simd_broadcast(score, 0);
for (uint d = lane; d < dk; d += simdWidth) out[d] += score * value[d];`,
		MeasuredWin: `On Apple M2 Pro, the issue #725 lane-striped decode path
improved sk=1/2/4/8 by 1.609x/1.545x/1.537x/1.258x with unchanged 8 B/op and
one allocation. Retained sk=16 and sk=128 stayed within 0.5%. Ten independent
alternating TinyLlama Q4_K_M pairs improved complete positions 0+1 from 84.163
ms to 78.298 ms (1.075x), with identical output bits; all nine affected
positions had maximum absolute logit error 3.20e-5.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6053",
		Doc:  "GPU kernel replicates and shuffles a full-vector accumulator in every lane",
		Run:  runPS6053,
	},
})

var (
	ps6053MetalKernelDecl  = regexp.MustCompile(`\bkernel\b[^;{}()]*?\b([A-Za-z_]\w*)\s*\(`)
	ps6053NativeKernelDecl = regexp.MustCompile(`\b(?:__global__|__kernel)\b[^;{}()]*?\b([A-Za-z_]\w*)\s*\(`)
	ps6053MainKernelDecl   = regexp.MustCompile(`\bvoid\s+(main)\s*\(`)
	ps6053FixedArray       = regexp.MustCompile(`\b(?:half|float|double|bfloat|int|uint|short|ushort|long|ulong|int32_t|uint32_t)(?:[2-4])?\s+([A-Za-z_]\w*)\s*\[\s*([A-Za-z_]\w*|[0-9]+)\s*\]`)
	ps6053ForHeader        = regexp.MustCompile(`\bfor\s*\(\s*(?:(?:const\s+)?(?:uint|int|uint32_t|int32_t|size_t|ushort|long)\s+)?([A-Za-z_]\w*)\s*=\s*0(?:[uUlL]*)\s*;\s*([A-Za-z_]\w*)\s*<\s*([A-Za-z_]\w*|[0-9]+)(?:[uUlL]*)\s*;\s*([^)]+)\)`)
	ps6053Shuffle          = regexp.MustCompile(`\b(?:simd_shuffle(?:_down)?|quad_shuffle(?:_down)?|__shfl_down_sync|__shfl_down|subgroupShuffleDown|sub_group_shuffle_down)\s*\(`)
	ps6053LaneZeroGuard    = regexp.MustCompile(`\bif\s*\([^)]*(?:lane|simd|threadIdx\s*\.\s*x|gl_SubgroupInvocationID)[^)]*(?:==\s*0|!=\s*[1-9]|!\s*[A-Za-z_]\w*)[^)]*\)\s*$`)
)

type ps6053Finding struct {
	kernel string
	array  string
	bound  string
	offset int
}

type ps6053Kernel struct {
	name       string
	start, end int
}

type ps6053Loop struct {
	index      string
	bound      string
	start, end int
	body       string
}

func runPS6053(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			source, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, finding := range ps6053ReplicatedAccumulatorKernels(source) {
				pass.Report(analysis.Diagnostic{
					Pos:     lit.Pos(),
					End:     lit.End(),
					Message: ps6053Message(finding),
				})
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
		findings := ps6053ReplicatedAccumulatorKernels(string(source))
		if len(findings) == 0 {
			continue
		}
		file := pass.Fset.AddFile(filename, -1, len(source))
		file.SetLinesForContent(source)
		for _, finding := range findings {
			offset := min(max(finding.offset, 0), len(source))
			pass.Report(analysis.Diagnostic{
				Pos:     file.Pos(offset),
				Message: ps6053Message(finding),
			})
		}
	}
	return nil, nil
}

func ps6053Message(finding ps6053Finding) string {
	return "GPU kernel " + finding.kernel + " keeps full-vector accumulator " + finding.array + "[" + finding.bound + "] private in every SIMD lane and shuffles every element, inflating register pressure — evaluate lane-owned vector-dimension striping with the smaller reduction axis streamed/broadcast; retain a measured shape crossover and gate numerical order, retained shapes, allocations, exact parity, and end-to-end performance (advisory)"
}

func ps6053ReadFile(pass *analysis.Pass, filename string) ([]byte, error) {
	if pass.ReadFile != nil {
		return pass.ReadFile(filename)
	}
	return os.ReadFile(filename)
}

func ps6053NativeExtension(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".metal", ".cu", ".cuh", ".hip", ".comp", ".compute", ".glsl", ".cl", ".c", ".cc", ".cpp", ".h", ".hpp", ".m", ".mm":
		return true
	default:
		return false
	}
}

func ps6053ReplicatedAccumulatorKernels(source string) []ps6053Finding {
	clean := ps6053BlankCommentsAndStrings(source)
	if !ps6053LooksGPU(clean) {
		return nil
	}
	kernels := ps6053Kernels(clean)
	var findings []ps6053Finding
	for _, kernel := range kernels {
		body := clean[kernel.start:kernel.end]
		if !ps6053SubgroupMapped(body) {
			continue
		}
		loops := ps6053Loops(body)
		for _, match := range ps6053FixedArray.FindAllStringSubmatchIndex(body, -1) {
			name := body[match[2]:match[3]]
			bound := body[match[4]:match[5]]
			if !ps6053AccumulatorName(name) || !ps6053FullVectorBound(bound) || !ps6053LanePrivate(body, match[0]) {
				continue
			}
			updated, shuffled := false, false
			for _, loop := range loops {
				if !strings.EqualFold(loop.bound, bound) || ps6053SingleLaneGuarded(body, loop.start) {
					continue
				}
				access := ps6053ArrayIndex(name, loop.index)
				if !access.MatchString(loop.body) {
					continue
				}
				if ps6053Shuffle.MatchString(loop.body) {
					shuffled = true
				} else if ps6053ArrayUpdated(loop.body, access) {
					updated = true
				}
			}
			if updated && shuffled {
				findings = append(findings, ps6053Finding{
					kernel: kernel.name,
					array:  name,
					bound:  bound,
					offset: kernel.start + match[0],
				})
			}
		}
	}
	return findings
}

func ps6053LooksGPU(source string) bool {
	metal := strings.Contains(source, "metal_stdlib") || strings.Contains(source, "thread_index_in_simdgroup") || strings.Contains(source, "simd_shuffle")
	cudaHIP := strings.Contains(source, "__global__") && (strings.Contains(source, "threadIdx") || strings.Contains(source, "__shfl"))
	vulkan := strings.Contains(source, "gl_SubgroupInvocationID") || strings.Contains(source, "subgroupShuffle")
	openCL := strings.Contains(source, "__kernel") && (strings.Contains(source, "get_sub_group_local_id") || strings.Contains(source, "sub_group_shuffle"))
	return metal || cudaHIP || vulkan || openCL
}

func ps6053SubgroupMapped(source string) bool {
	lane := ps6007ContainsAny(source, "thread_index_in_simdgroup", "threadIdx.x", "gl_SubgroupInvocationID", "get_sub_group_local_id")
	group := ps6007ContainsAny(source, "threadgroup_position_in_grid", "simdgroup_index_in_threadgroup", "blockIdx.", "threadIdx.y", "gl_WorkGroupID", "get_group_id")
	return lane && group
}

func ps6053Kernels(source string) []ps6053Kernel {
	type declaration struct {
		name       string
		start, end int
	}
	var declarations []declaration
	for _, pattern := range []*regexp.Regexp{ps6053MetalKernelDecl, ps6053NativeKernelDecl, ps6053MainKernelDecl} {
		for _, match := range pattern.FindAllStringSubmatchIndex(source, -1) {
			declarations = append(declarations, declaration{name: source[match[2]:match[3]], start: match[0], end: match[1]})
		}
	}
	slices.SortFunc(declarations, func(left, right declaration) int { return left.start - right.start })
	kernels := make([]ps6053Kernel, 0, len(declarations))
	lastStart := -1
	for _, decl := range declarations {
		if decl.start == lastStart {
			continue
		}
		lastStart = decl.start
		openRelative := strings.IndexByte(source[decl.end:], '{')
		if openRelative < 0 {
			continue
		}
		open := decl.end + openRelative
		close := ps6053MatchingBrace(source, open)
		if close < 0 {
			continue
		}
		kernels = append(kernels, ps6053Kernel{name: decl.name, start: decl.start, end: close + 1})
	}
	return kernels
}

func ps6053Loops(source string) []ps6053Loop {
	var loops []ps6053Loop
	for _, match := range ps6053ForHeader.FindAllStringSubmatchIndex(source, -1) {
		if source[match[2]:match[3]] != source[match[4]:match[5]] || !ps6053UnitStep(source[match[2]:match[3]], source[match[8]:match[9]]) {
			continue
		}
		start := match[0]
		bodyStart := match[1]
		for bodyStart < len(source) && (source[bodyStart] == ' ' || source[bodyStart] == '\t' || source[bodyStart] == '\r' || source[bodyStart] == '\n') {
			bodyStart++
		}
		bodyEnd := bodyStart
		if bodyStart < len(source) && source[bodyStart] == '{' {
			close := ps6053MatchingBrace(source, bodyStart)
			if close < 0 {
				continue
			}
			bodyStart++
			bodyEnd = close
		} else if semicolon := strings.IndexByte(source[bodyStart:], ';'); semicolon >= 0 {
			bodyEnd = bodyStart + semicolon + 1
		} else {
			continue
		}
		loops = append(loops, ps6053Loop{
			index: source[match[2]:match[3]],
			bound: source[match[6]:match[7]],
			start: start,
			end:   bodyEnd,
			body:  source[bodyStart:bodyEnd],
		})
	}
	return loops
}

func ps6053UnitStep(index, step string) bool {
	step = strings.ReplaceAll(step, " ", "")
	step = strings.ReplaceAll(step, "\t", "")
	return step == index+"++" || step == "++"+index || step == index+"+=1" || step == index+"="+index+"+1"
}

func ps6053ArrayIndex(array, index string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(array) + `\s*\[\s*` + regexp.QuoteMeta(index) + `\s*\]`)
}

func ps6053ArrayUpdated(body string, access *regexp.Regexp) bool {
	for _, location := range access.FindAllStringIndex(body, -1) {
		tail := strings.TrimSpace(body[location[1]:])
		if strings.HasPrefix(tail, "=") || strings.HasPrefix(tail, "+=") || strings.HasPrefix(tail, "-=") || strings.HasPrefix(tail, "*=") || strings.HasPrefix(tail, "/=") || strings.HasPrefix(tail, "++") || strings.HasPrefix(tail, "--") {
			return true
		}
	}
	return false
}

func ps6053AccumulatorName(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "acc", "accumulator", "partialsum", "outputsum", "resultsum")
}

func ps6053FullVectorBound(bound string) bool {
	if value, err := strconv.Atoi(bound); err == nil {
		return value >= 32
	}
	bound = ps6007NormalizeName(bound)
	return ps6007ContainsAny(bound, "dk", "headdim", "headsize", "vectordim", "outputdim", "valuedim", "embeddingdim")
}

func ps6053LanePrivate(source string, offset int) bool {
	lineStart := strings.LastIndexByte(source[:offset], '\n') + 1
	prefix := ps6007NormalizeName(source[lineStart:offset])
	return !ps6007ContainsAny(prefix, "threadgroup", "shared", "groupshared", "workgroup")
}

func ps6053SingleLaneGuarded(source string, offset int) bool {
	stack := make([]bool, 0, 8)
	for index := 0; index < offset; index++ {
		switch source[index] {
		case '{':
			start := max(strings.LastIndexAny(source[:index], ";{}")+1, 0)
			stack = append(stack, ps6053LaneZeroGuard.MatchString(strings.TrimSpace(source[start:index])))
		case '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return slices.Contains(stack, true)
}

func ps6053MatchingBrace(source string, open int) int {
	depth := 0
	for index := open; index < len(source); index++ {
		switch source[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

// ps6053BlankCommentsAndStrings preserves byte offsets and newlines while
// removing C-family comments and quoted content from consideration.
func ps6053BlankCommentsAndStrings(source string) string {
	out := []byte(source)
	for index := 0; index < len(out); {
		switch {
		case out[index] == '/' && index+1 < len(out) && out[index+1] == '/':
			for index < len(out) && out[index] != '\n' {
				out[index] = ' '
				index++
			}
		case out[index] == '/' && index+1 < len(out) && out[index+1] == '*':
			out[index], out[index+1] = ' ', ' '
			index += 2
			for index < len(out) {
				if index+1 < len(out) && out[index] == '*' && out[index+1] == '/' {
					out[index], out[index+1] = ' ', ' '
					index += 2
					break
				}
				if out[index] != '\n' {
					out[index] = ' '
				}
				index++
			}
		case out[index] == '"' || out[index] == '\'':
			quote := out[index]
			out[index] = ' '
			index++
			for index < len(out) {
				if out[index] == '\\' && index+1 < len(out) {
					out[index], out[index+1] = ' ', ' '
					index += 2
					continue
				}
				end := out[index] == quote
				if out[index] != '\n' {
					out[index] = ' '
				}
				index++
				if end {
					break
				}
			}
		default:
			index++
		}
	}
	return string(out)
}
