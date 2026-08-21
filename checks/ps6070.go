package checks

import (
	"go/ast"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6070 implements owner issue #787. It prices the redistribution work of a
// Metal packed-load candidate instead of treating fewer source loads as an
// automatic reduction in effective memory traffic.
var PS6070 = register(&lint.Check{
	ID:       "PS6070",
	Category: "verify",
	Slug:     "metal-packed-load-dynamic-simd-redistribution",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a partial-lane Metal packed load pays a dynamic SIMD redistribution on every block",
		Text: `Reducing the number of lanes that spell a device load does not
necessarily reduce effective memory work. Contiguous or duplicate byte reads
can already coalesce well, while a partial-lane packed-load candidate adds a
lane predicate, a dependency through the loading lanes, and a dynamic SIMD
shuffle or broadcast to every decoded block.

This check implements owner issue #787. It scans embedded Go string literals,
package-owned Metal sources, and Metal shaders stored as concatenated
C/Objective-C string literals. Inside a packed quant/dequant/unpack/decode or
qmatmul kernel it requires the complete candidate shape:

  - a thread_index_in_simdgroup lane parameter;
  - a zero-based, unit-step block loop;
  - a ushort-or-wider packed device load whose address depends on that lane;
  - a compile-time guard that restricts the load to 2 through 31 low lanes;
    and
  - simd_shuffle(value, dynamicLane) or simd_broadcast(value, dynamicLane)
    inside the same block loop.

The finding reports the statically visible active loader count, packed load
width, aggregate source span, and redistribution-call count per iteration. A
numeric constant source stays silent: lane-zero uniform-address broadcast is
the separate PS6068 shape. Uniform addresses, unguarded/all-lane loads,
single-leader loads, redistribution outside the block loop, ordinary
non-quant kernels, comments, quoted examples, and annotated validated kernels
also stay silent. A
//perfscan:packed-load-redistribution-validated annotation records an external
same-binary retention contract.

Keep the control and candidate in the same binary behind the identical route
predicate. Record active loader lanes, shuffles per iteration, packed source
span, the control's duplicate-address factor, and measured crossover versus
byte count. Compare alternating AB/BA end-to-end campaigns across every
eligible shape and require a predeclared retention floor; inspect native code
or device counters when available. Preserve exact decode indices, byte/nibble
order, scaling, accumulation order, output parity, and disabled-route zero
dispatches. Retain the simpler byte-load control when redistribution costs
more than any memory saving. There is NO automatic fix because load
coalescing, shuffle latency, crossover, and route profitability are
device/toolchain/runtime facts.`,
		Before: `ushort pair = 0;
if (lane < 8) {
	pair = *((device const ushort *)(weights + blockOffset + lane*2));
}
pair = simd_shuffle(pair, byteIndex >> 1);`,
		After: `// Keep the indexed-byte control and packed candidate selectable.
// Retain the packed path only above a measured same-binary crossover.
// Evidence: loader lanes, shuffles/block, source span, duplicate factor.`,
		MeasuredWin: `The owner Apple M2 Pro Q4_0 candidate used eight aligned
ushort loads per 32-weight block and one dynamic simd_shuffle in every lane,
but achieved only 1.022x, 0.681x, 0.589x, 0.590x, and 0.398x control across
the five tested K/N cells. It failed the predeclared 1.10x retention gate and
was reverted; the regression sharpened as byte count grew.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6070",
		Doc:  "partial-lane Metal packed load performs a dynamic SIMD redistribution in every quant block",
		Run:  runPS6070,
	},
})

var (
	ps6070RedistributionCall = regexp.MustCompile(`\b(simd_shuffle|simd_broadcast)\s*\(`)
	ps6070PackedCast         = regexp.MustCompile(`(?i)\bdevice\s+const\s+(u?short[2-4]?|u?int[2-4]?|u?long[2-4]?|half[2-4]?|float[2-4]?)\s*\*`)
	ps6070PackedPointer      = regexp.MustCompile(`(?i)\bdevice\s+const\s+(u?short[2-4]?|u?int[2-4]?|u?long[2-4]?|half[2-4]?|float[2-4]?)\s*\*\s*([A-Za-z_]\w*)`)
	ps6070NumericCast        = regexp.MustCompile(`(?i)\(\s*(?:u?char|u?short|u?int|u?long|size_t|int(?:8|16|32|64)_t|uint(?:8|16|32|64)_t)\s*\)`)
	ps6070NumericLiteral     = regexp.MustCompile(`(?i)\b(?:0x[0-9a-f]+|[0-9]+)[uUlL]*\b`)
)

type ps6070Redistribution struct {
	name   string
	value  string
	source string
	offset int
}

type ps6070Finding struct {
	kernel       string
	loaders      int
	loadWidth    int
	sourceSpan   int
	redistribute int
	offset       int
}

func runPS6070(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			source, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			for _, finding := range ps6070PackedRedistributions(source) {
				pass.Report(analysis.Diagnostic{Pos: literal.Pos(), End: literal.End(), Message: ps6070Message(finding)})
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
		findings := ps6070PackedRedistributions(string(source))
		if len(findings) == 0 {
			continue
		}
		file := pass.Fset.AddFile(filename, -1, len(source))
		file.SetLinesForContent(source)
		for _, finding := range findings {
			offset := min(max(finding.offset, 0), len(source))
			pass.Reportf(file.Pos(offset), "%s", ps6070Message(finding))
		}
	}
	return nil, nil
}

func ps6070PackedRedistributions(source string) []ps6070Finding {
	if !strings.Contains(source, "thread_index_in_simdgroup") ||
		!ps6007ContainsAny(source, "simd_shuffle", "simd_broadcast") {
		return nil
	}
	findings := ps6070DirectPackedRedistributions(source)
	for _, fragment := range ps6068ConcatenatedCStringFragments(source) {
		for _, finding := range ps6070DirectPackedRedistributions(fragment.source) {
			finding.offset = fragment.offset
			findings = append(findings, finding)
		}
	}
	return findings
}

func ps6070DirectPackedRedistributions(source string) []ps6070Finding {
	clean := ps6053BlankCommentsAndStrings(source)
	if !ps6053LooksGPU(clean) {
		return nil
	}
	var findings []ps6070Finding
	for _, kernel := range ps6053Kernels(clean) {
		if !ps6069QuantContext(kernel.name) || ps6070Validated(source, kernel) {
			continue
		}
		body := clean[kernel.start:kernel.end]
		laneMatch := ps6068MetalLane.FindStringSubmatch(body)
		if laneMatch == nil {
			continue
		}
		lane := laneMatch[1]
		dependent := ps6068LaneDependentObjects(body, lane)
		packedPointers := ps6070PackedPointers(body)
		for _, loop := range ps6053Loops(body) {
			redistributions := ps6070RedistributionCalls(loop.body)
			finding := ps6070Finding{kernel: kernel.name, offset: kernel.start + loop.start}
			for _, redistribution := range redistributions {
				if ps6070NumericSource(redistribution.source) {
					continue
				}
				base := ps6070BaseValue(redistribution.value)
				assignment, rhs, ok := ps6068LastAssignment(loop.body, base, redistribution.offset)
				if !ok || !ps6068DeviceLoad(rhs) ||
					!ps6068ExpressionDependsOn(rhs, lane, dependent) {
					continue
				}
				loadWidth := ps6070PackedLoadWidth(rhs, packedPointers)
				if loadWidth < 2 {
					continue
				}
				loaders, ok := ps6070LoaderGuard(loop.body, assignment, lane)
				if !ok || loaders <= 1 || loaders >= 32 {
					continue
				}
				if finding.redistribute == 0 {
					finding.loaders = loaders
					finding.loadWidth = loadWidth
					finding.sourceSpan = loaders * loadWidth
				}
				finding.redistribute++
			}
			if finding.redistribute != 0 {
				findings = append(findings, finding)
				// One high-signal advisory per kernel is enough even if the
				// decoded block has several packed planes or nested loops.
				break
			}
		}
	}
	return findings
}

func ps6070RedistributionCalls(body string) []ps6070Redistribution {
	var calls []ps6070Redistribution
	for _, match := range ps6070RedistributionCall.FindAllStringSubmatchIndex(body, -1) {
		open := match[1] - 1
		close := ps6069MatchingParen(body, open)
		if close < 0 {
			continue
		}
		arguments := ps6069SplitTopLevel(body[open+1:close], ",")
		if len(arguments) != 2 {
			continue
		}
		calls = append(calls, ps6070Redistribution{
			name:   body[match[2]:match[3]],
			value:  strings.TrimSpace(arguments[0]),
			source: strings.TrimSpace(arguments[1]),
			offset: match[0],
		})
	}
	return calls
}

func ps6070NumericSource(source string) bool {
	source = ps6070NumericCast.ReplaceAllString(source, "")
	stripped := ps6070NumericLiteral.ReplaceAllString(source, "")
	stripped = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n', '(', ')', '+', '-', '*', '/', '%', '<', '>', '&', '|', '^', '~', '!':
			return -1
		default:
			return r
		}
	}, stripped)
	return stripped == ""
}

func ps6070BaseValue(value string) string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '.' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func ps6070PackedPointers(body string) map[string]int {
	pointers := make(map[string]int)
	for _, match := range ps6070PackedPointer.FindAllStringSubmatch(body, -1) {
		if width := ps6070MetalTypeWidth(match[1]); width >= 2 {
			pointers[match[2]] = width
		}
	}
	return pointers
}

func ps6070PackedLoadWidth(rhs string, pointers map[string]int) int {
	if match := ps6070PackedCast.FindStringSubmatch(rhs); match != nil {
		return ps6070MetalTypeWidth(match[1])
	}
	for name, width := range pointers {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*\[`).MatchString(rhs) {
			return width
		}
	}
	return 0
}

func ps6070MetalTypeWidth(name string) int {
	name = strings.ToLower(name)
	vector := 1
	if last := name[len(name)-1]; last >= '2' && last <= '4' {
		vector = int(last - '0')
		name = name[:len(name)-1]
	}
	component := 0
	switch name {
	case "short", "ushort", "half":
		component = 2
	case "int", "uint", "float":
		component = 4
	case "long", "ulong":
		component = 8
	}
	return component * vector
}

func ps6070LoaderGuard(body string, offset int, lane string) (int, bool) {
	stack := make([]int, 0, 8)
	for index := 0; index < offset; index++ {
		switch body[index] {
		case '{':
			start := max(strings.LastIndexAny(body[:index], ";{}")+1, 0)
			loaders, _ := ps6070LoaderCondition(body[start:index], lane)
			stack = append(stack, loaders)
		case '}':
			if len(stack) != 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	for index := len(stack) - 1; index >= 0; index-- {
		if stack[index] != 0 {
			return stack[index], true
		}
	}
	start := max(strings.LastIndexAny(body[:offset], ";\n{}")+1, 0)
	return ps6070LoaderCondition(body[start:offset], lane)
}

func ps6070LoaderCondition(source, lane string) (int, bool) {
	source = strings.TrimSpace(source)
	if !strings.Contains(source, "if") {
		return 0, false
	}
	quotedLane := regexp.QuoteMeta(lane)
	patterns := []struct {
		pattern   *regexp.Regexp
		inclusive bool
	}{
		{pattern: regexp.MustCompile(`(?s)\bif\s*\([^)]*\b` + quotedLane + `\b\s*<\s*([0-9]+[uUlL]*)[^)]*\)\s*$`)},
		{pattern: regexp.MustCompile(`(?s)\bif\s*\([^)]*\b` + quotedLane + `\b\s*<=\s*([0-9]+[uUlL]*)[^)]*\)\s*$`), inclusive: true},
		{pattern: regexp.MustCompile(`(?s)\bif\s*\([^)]*([0-9]+[uUlL]*)\s*>\s*\b` + quotedLane + `\b[^)]*\)\s*$`)},
		{pattern: regexp.MustCompile(`(?s)\bif\s*\([^)]*([0-9]+[uUlL]*)\s*>=\s*\b` + quotedLane + `\b[^)]*\)\s*$`), inclusive: true},
	}
	for _, candidate := range patterns {
		match := candidate.pattern.FindStringSubmatch(source)
		if match == nil {
			continue
		}
		value, ok := ps6069Integer(match[1])
		if !ok || value < 0 || value > 32 {
			return 0, false
		}
		if candidate.inclusive {
			value++
		}
		return int(value), true
	}
	return 0, false
}

func ps6070Validated(source string, kernel ps6053Kernel) bool {
	start := max(kernel.start-256, 0)
	text := ps6058Compact(source[start:kernel.end])
	return strings.Contains(text, "perfscanpackedloadredistributionvalidated") ||
		strings.Contains(text, "perfscanpackedredistributionvalidated")
}

func ps6070Message(finding ps6070Finding) string {
	return "Metal packed-quant kernel " + finding.kernel + " restricts " +
		strconv.Itoa(finding.loadWidth) + "-byte device loads to " + strconv.Itoa(finding.loaders) +
		" of 32 lanes (" + strconv.Itoa(finding.sourceSpan) + "-byte packed source span) and executes " +
		strconv.Itoa(finding.redistribute) + " dynamic SIMD redistribution call(s) per block iteration; fewer source loads do not prove less effective memory work when the byte control can coalesce, while the lane predicate and shuffle dependency can dominate — record active loaders, shuffles/iteration, source span, control duplicate factor, and measured crossover versus byte count in a same-binary alternating route-gated campaign, retaining the byte-load control below the proved crossover (advisory, no automatic fix)"
}
