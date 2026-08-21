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

// PS6068 implements owner issue #785. It recognizes Metal kernels that replace
// a lane-uniform device load with one leader load and an explicit SIMD
// broadcast. That source-level load reduction is a hypothesis, not a proven
// reduction in effective memory work.
var PS6068 = register(&lint.Check{
	ID:       "PS6068",
	Category: "verify",
	Slug:     "metal-uniform-load-explicit-simd-broadcast",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a lane-uniform Metal load is serialized through an explicit SIMD broadcast",
		Text: `A device address can be identical in every SIMD lane even when the
source spells the load once per lane. The compiler, cache hierarchy, or load
hardware may already exploit that uniformity. Replacing it with a lane-zero
load plus simd_broadcast_first can therefore add leader serialization and
shuffle pressure without removing effective memory work.

This check implements owner issue #785. It scans embedded Go string literals
and package-owned native GPU sources. Inside a Metal kernel it requires the
complete source shape:

  - a thread_index_in_simdgroup lane parameter;
  - a local value assigned from an indexed or pointer-based device load only
    under a lane-zero/simd_is_first guard;
  - simd_broadcast_first(value) or simd_broadcast(value, 0); and
  - a load expression whose address does not depend on the lane, including
    through simple native local assignments.

The finding is aggregated once per kernel even when a packed header is
broadcast component by component. Lane-dependent addresses, values computed
without a device load, unguarded loads, nonzero broadcast sources, comments,
quoted examples, and annotated validated kernels stay silent. A
//perfscan:simd-uniform-load-broadcast-validated annotation records an external
same-binary retention contract.

Keep the candidate separately selectable and default-off. Compare control and
candidate in the same binary with alternating AB/BA order, steady repeated
dispatches, identical route/geometry/allocation/quality boundaries, and a
predeclared end-to-end retention floor across the eligible shape matrix. Do not
infer a win from source load counts. Inspect generated native code or counters
when available; compiler and hardware uniform-load handling may already remove
the apparent redundancy.

Before changing byte-addressed device loads to ushort, uint, vector, or other
wider pointer loads, prove base, row-stride, block-stride, and modulo alignment
for every routed shape. Preserve odd/alternating alignments and planted finite,
NaN, and infinity parity. There is NO automatic fix because effective load
coalescing, shuffle cost, pointer alignment, route coverage, and profitable
shapes are device/toolchain/runtime facts.`,
		Before: `ushort header = 0;
if (lane == 0) {
	header = *((device const ushort *)(weights + byteOffset));
}
header = simd_broadcast_first(header);`,
		After: `// Keep the lane-local control and this candidate separately selectable.
// Retain the broadcast only after same-binary AB/BA end-to-end evidence passes.
// Prove every byteOffset alignment before using a widened device pointer.`,
		MeasuredWin: `In the owner Apple M2 Pro Q3_K pilot, the explicit
lane-zero load/broadcast candidate reduced an apparent 448 byte reads per SIMD
group/superblock to seven ushort loads and four broadcasts, yet failed every
predeclared >=1.10x retention gate. Candidate/control ranges were 0.926-1.024x
across seven K/N cells; the larger-K cells consistently regressed. Q3_K's
110-byte block and row stride allowed two-byte loads but not four-byte
device-pointer loads.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6068",
		Doc:  "Metal lane-uniform device load is serialized through an explicit SIMD broadcast without performance proof",
		Run:  runPS6068,
	},
})

var (
	ps6068MetalLane      = regexp.MustCompile(`\b([A-Za-z_]\w*)\s*\[\[\s*thread_index_in_simdgroup\s*\]\]`)
	ps6068BroadcastFirst = regexp.MustCompile(`\bsimd_broadcast_first\s*\(\s*([A-Za-z_]\w*(?:\s*\.\s*[A-Za-z_]\w*)?)\s*\)`)
	ps6068BroadcastZero  = regexp.MustCompile(`\bsimd_broadcast\s*\(\s*([A-Za-z_]\w*(?:\s*\.\s*[A-Za-z_]\w*)?)\s*,\s*0(?:[uUlL]*)\s*\)`)
	ps6068Assignment     = regexp.MustCompile(`(?m)\b([A-Za-z_]\w*)\s*=\s*([^;\n]+)`)
	ps6068IndexedLoad    = regexp.MustCompile(`\b[A-Za-z_]\w*\s*\[[^\]]+\]`)
	ps6068LoadCall       = regexp.MustCompile(`(?i)\b(?:load|read|fetch)[A-Za-z_0-9]*\s*\(`)
	ps6068WideDeviceCast = regexp.MustCompile(`(?i)device\s+const\s+(?:u?short[2-4]?|u?int[2-4]?|u?long[2-4]?|half[2-4]?|float[2-4]?)\s*\*`)
)

type ps6068Broadcast struct {
	value  string
	offset int
}

type ps6068Finding struct {
	kernel  string
	values  []string
	widened bool
	offset  int
}

type ps6068SourceFragment struct {
	source string
	offset int
}

func runPS6068(pass *analysis.Pass) (any, error) {
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
			for _, finding := range ps6068UniformLoadBroadcasts(source) {
				pass.Report(analysis.Diagnostic{Pos: literal.Pos(), End: literal.End(), Message: ps6068Message(finding)})
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
		findings := ps6068UniformLoadBroadcasts(string(source))
		if len(findings) == 0 {
			continue
		}
		file := pass.Fset.AddFile(filename, -1, len(source))
		file.SetLinesForContent(source)
		for _, finding := range findings {
			offset := min(max(finding.offset, 0), len(source))
			pass.Reportf(file.Pos(offset), "%s", ps6068Message(finding))
		}
	}
	return nil, nil
}

func ps6068UniformLoadBroadcasts(source string) []ps6068Finding {
	if !strings.Contains(source, "thread_index_in_simdgroup") ||
		!ps6007ContainsAny(source, "simd_broadcast_first", "simd_broadcast") {
		return nil
	}
	findings := ps6068DirectUniformLoadBroadcasts(source)
	for _, fragment := range ps6068ConcatenatedCStringFragments(source) {
		for _, finding := range ps6068DirectUniformLoadBroadcasts(fragment.source) {
			// C escape decoding changes byte offsets. Point at the first source
			// literal in the concatenated shader instead of inventing precision.
			finding.offset = fragment.offset
			findings = append(findings, finding)
		}
	}
	return findings
}

func ps6068DirectUniformLoadBroadcasts(source string) []ps6068Finding {
	clean := ps6053BlankCommentsAndStrings(source)
	if !strings.Contains(clean, "thread_index_in_simdgroup") ||
		!ps6007ContainsAny(clean, "metal_stdlib", "simd_broadcast_first", "simd_broadcast") {
		return nil
	}
	var findings []ps6068Finding
	for _, kernel := range ps6053Kernels(clean) {
		body := clean[kernel.start:kernel.end]
		laneMatch := ps6068MetalLane.FindStringSubmatch(body)
		if laneMatch == nil || ps6068Validated(source, kernel) {
			continue
		}
		lane := laneMatch[1]
		dependent := ps6068LaneDependentObjects(body, lane)
		broadcasts := ps6068BroadcastCalls(body)
		seenValues := make(map[string]bool, len(broadcasts))
		finding := ps6068Finding{kernel: kernel.name, offset: kernel.start}
		for _, broadcast := range broadcasts {
			base := strings.FieldsFunc(broadcast.value, func(r rune) bool { return r == '.' || r == ' ' || r == '\t' })[0]
			if seenValues[base] {
				continue
			}
			assignment, rhs, ok := ps6068LastAssignment(body, base, broadcast.offset)
			if !ok || !ps6068DeviceLoad(rhs) || ps6068ExpressionDependsOn(rhs, lane, dependent) ||
				!ps6068LeaderOnly(body, assignment, lane) {
				continue
			}
			seenValues[base] = true
			finding.values = append(finding.values, base)
			finding.widened = finding.widened || ps6068WideDeviceCast.MatchString(rhs)
			if len(finding.values) == 1 {
				finding.offset = kernel.start + broadcast.offset
			}
		}
		if len(finding.values) != 0 {
			findings = append(findings, finding)
		}
	}
	return findings
}

func ps6068ConcatenatedCStringFragments(source string) []ps6068SourceFragment {
	var fragments []ps6068SourceFragment
	parts := make([]string, 0, 64)
	blockStart, previousEnd := -1, -1
	flush := func() {
		if len(parts) != 0 {
			fragments = append(fragments, ps6068SourceFragment{source: strings.Join(parts, ""), offset: blockStart})
		}
		parts = parts[:0]
		blockStart, previousEnd = -1, -1
	}
	for index := 0; index < len(source); {
		switch {
		case source[index] == '/' && index+1 < len(source) && source[index+1] == '/':
			if newline := strings.IndexByte(source[index+2:], '\n'); newline >= 0 {
				index += newline + 2
			} else {
				index = len(source)
			}
		case source[index] == '/' && index+1 < len(source) && source[index+1] == '*':
			if close := strings.Index(source[index+2:], "*/"); close >= 0 {
				index += close + 4
			} else {
				index = len(source)
			}
		case source[index] == '\'':
			end := ps6068QuotedEnd(source, index, '\'')
			if end < 0 {
				index = len(source)
			} else {
				index = end
			}
		case source[index] == '"':
			end := ps6068QuotedEnd(source, index, '"')
			if end < 0 {
				index = len(source)
				continue
			}
			decoded, err := strconv.Unquote(source[index:end])
			if err != nil {
				flush()
				index = end
				continue
			}
			if previousEnd >= 0 && !ps6068OnlyTrivia(source[previousEnd:index]) {
				flush()
			}
			if blockStart < 0 {
				blockStart = index
			}
			parts = append(parts, decoded)
			previousEnd = end
			index = end
		default:
			index++
		}
	}
	flush()
	return fragments
}

func ps6068QuotedEnd(source string, start int, quote byte) int {
	for index := start + 1; index < len(source); index++ {
		if source[index] == '\\' {
			index++
			continue
		}
		if source[index] == quote {
			return index + 1
		}
	}
	return -1
}

func ps6068OnlyTrivia(source string) bool {
	return strings.TrimSpace(ps6053BlankCommentsAndStrings(source)) == ""
}

func ps6068BroadcastCalls(body string) []ps6068Broadcast {
	var result []ps6068Broadcast
	for _, pattern := range []*regexp.Regexp{ps6068BroadcastFirst, ps6068BroadcastZero} {
		for _, match := range pattern.FindAllStringSubmatchIndex(body, -1) {
			result = append(result, ps6068Broadcast{value: body[match[2]:match[3]], offset: match[0]})
		}
	}
	slices.SortFunc(result, func(left, right ps6068Broadcast) int { return left.offset - right.offset })
	return result
}

func ps6068LastAssignment(body, value string, before int) (int, string, bool) {
	pattern := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(value) + `(?:\s*\.\s*[A-Za-z_]\w*)?\s*=\s*([^;\n]+)`)
	var offset int
	var rhs string
	found := false
	for _, match := range pattern.FindAllStringSubmatchIndex(body[:before], -1) {
		candidate := body[match[2]:match[3]]
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		offset = match[0]
		rhs = candidate
		found = true
	}
	return offset, rhs, found
}

func ps6068DeviceLoad(expression string) bool {
	return ps6068IndexedLoad.MatchString(expression) ||
		strings.Contains(expression, "device") && strings.IndexByte(expression, '*') >= 0 ||
		ps6068LoadCall.MatchString(expression)
}

func ps6068LaneDependentObjects(body, lane string) map[string]bool {
	dependent := map[string]bool{lane: true}
	changed := true
	for changed {
		changed = false
		for _, match := range ps6068Assignment.FindAllStringSubmatch(body, -1) {
			name, expression := match[1], match[2]
			if dependent[name] || !ps6068ExpressionDependsOn(expression, lane, dependent) {
				continue
			}
			dependent[name] = true
			changed = true
		}
	}
	return dependent
}

func ps6068ExpressionDependsOn(expression, lane string, dependent map[string]bool) bool {
	if ps6068ContainsWord(expression, lane) ||
		ps6007ContainsAny(expression, "thread_index_in_simdgroup", "simd_lane_id", "simdgroup_lane") {
		return true
	}
	for name := range dependent {
		if name != lane && ps6068ContainsWord(expression, name) {
			return true
		}
	}
	return false
}

func ps6068ContainsWord(source, word string) bool {
	if word == "" {
		return false
	}
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`).MatchString(source)
}

func ps6068LeaderOnly(body string, offset int, lane string) bool {
	stack := make([]bool, 0, 8)
	for index := 0; index < offset; index++ {
		switch body[index] {
		case '{':
			start := max(strings.LastIndexAny(body[:index], ";{}")+1, 0)
			stack = append(stack, ps6068LeaderCondition(body[start:index], lane))
		case '}':
			if len(stack) != 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if slices.Contains(stack, true) {
		return true
	}
	start := max(strings.LastIndexAny(body[:offset], ";\n{}")+1, 0)
	if ps6068LeaderCondition(body[start:offset], lane) {
		return true
	}
	prefix := body[:offset]
	earlyExit := regexp.MustCompile(`(?s)\bif\s*\(\s*(?:` + regexp.QuoteMeta(lane) + `\s*!=\s*0|0\s*!=\s*` + regexp.QuoteMeta(lane) + `)\s*\)\s*(?:\{\s*)?return\s*;`)
	return earlyExit.MatchString(prefix)
}

func ps6068LeaderCondition(source, lane string) bool {
	if strings.Contains(ps6007NormalizeName(source), "ifsimdisfirst") {
		return true
	}
	condition := regexp.MustCompile(`(?s)\bif\s*\([^)]*(?:\b` + regexp.QuoteMeta(lane) + `\b\s*==\s*0|0\s*==\s*\b` + regexp.QuoteMeta(lane) + `\b|!\s*\b` + regexp.QuoteMeta(lane) + `\b|simd_is_first\s*\(\s*\))[^)]*\)\s*$`)
	return condition.MatchString(strings.TrimSpace(source))
}

func ps6068Validated(source string, kernel ps6053Kernel) bool {
	start := max(kernel.start-256, 0)
	text := ps6058Compact(source[start:kernel.end])
	return strings.Contains(text, "perfscansimduniformloadbroadcastvalidated") ||
		strings.Contains(text, "perfscanuniformloadbroadcastvalidated")
}

func ps6068Message(finding ps6068Finding) string {
	values := strings.Join(finding.values, ", ")
	alignment := "prove base/row/block alignment before any widened device-pointer access"
	if finding.widened {
		alignment = "the widened device-pointer load requires an explicit base/row/block alignment proof"
	}
	return "Metal kernel " + finding.kernel + " loads lane-uniform device value(s) " + values + " only in the SIMD leader and explicitly broadcasts them; fewer source loads do not prove less effective memory work because compiler/hardware uniform-load handling may already coalesce the control, while leader serialization and shuffle pressure can regress — keep a separately selectable default-off candidate, require same-binary alternating end-to-end shape evidence against a predeclared retention floor, and " + alignment + " (advisory, no automatic fix)"
}
