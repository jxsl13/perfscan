package checks

import (
	"go/ast"
	"go/token"
	"math/bits"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6069 implements owner issue #786. It proves the maximum power-of-two load
// width of a nested native byte loop from the complete assignment/stride chain.
var PS6069 = register(&lint.Check{
	ID:       "PS6069",
	Category: "verify",
	Slug:     "metal-packed-quant-load-width-from-full-stride",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an eight-byte Metal quant load is aligned for two scalar words but not one vector word",
		Text: `A loop that consumes eight adjacent bytes does not automatically
permit one eight-byte device load. The maximum legal load width is constrained
by the base pointer, row and block strides, quant-plane/group/lane offsets, and
the loop start. An odd multiple of four anywhere in that chain can preserve
uint alignment while breaking uint2/ulong alignment.

This check implements owner issue #786. It scans embedded Go strings,
package-owned Metal sources, and Metal shaders stored as concatenated
C/Objective-C string literals. It reports a deliberately narrow shape:

  - a packed quant/dequant/unpack/decode/matmul Metal kernel reads a device
    const uchar/char byte source;
  - an inner loop covers exactly eight adjacent byte indices from that source;
  - the loop is nested inside another loop; and
  - recursive modular evaluation of native local assignments proves the first
    byte offset divisible by four, but cannot prove it divisible by eight.

The evaluator follows simple integer locals through casts, addition,
subtraction, and multiplication. Multiplying by an 84-byte block stride, for
example, proves two trailing zero bits regardless of the symbolic block index;
adding 16-, 32-, and 8-byte nested offsets preserves that four-byte alignment
but cannot create an eight-byte guarantee. Lane-dependent or symbolic terms are
treated conservatively. Eight-byte-aligned spans, shorter loops, non-byte
sources, nonnested loops, ordinary non-quant kernels, comments, and annotated
validated kernels stay silent.

Use exactly two scalar uint loads and extract the historical eight bytes in
registers only as a separately selectable candidate. Prove the actual device
base alignment and every routed row/block/plane/group/lane offset. Do not widen
to uint2, ulong, or a wider device pointer when the full stride chain only
guarantees four bytes. Preserve byte order, shift/scale/min unpacking, input
indices, accumulation order, odd rows, and alternating modulo-eight alignment.

Retain the historical pipeline below a measured crossover. Require shared
direct/resident/Recorder route predicates, zero candidate dispatches for
disabled/M>1/below-threshold shapes, planted finite/NaN/Inf and immutability
parity, and repeated same-binary AB/BA campaigns across eligible shapes. There
is NO automatic fix because native pointer alignment, shader syntax, route
selection, and the profitable K*N crossover are project/device facts.`,
		Before: `for (short l = laneBase; l < laneBase+8; l++) {
	uchar q = weights[quantBase+groupOffset+l];
	accumulate(q);
}`,
		After: `// Only after proving the full offset is 4-byte but not 8-byte aligned:
uint q0 = *((device const uint *)(weights + quantBase + groupOffset + laneBase));
uint q1 = *((device const uint *)(weights + quantBase + groupOffset + laneBase + 4));
// Extract the same eight bytes; retain only above the measured route threshold.`,
		MeasuredWin: `In three independent same-binary count-7 Apple M2 Pro
campaigns, the owner Q2_K candidate improved eligible resident-decode cells by
1.300x-1.509x at K2048,N3072, 1.360x-1.365x at K4096,N2048,
1.444x-1.451x at K2048,N5632, 1.457x-1.498x at K5632,N2048, and
1.750x-1.788x at K2048,N32000. The retained crossover was M=1 and
K*N>=6,291,456. The 84-byte block/row stride is divisible by four but not
eight, so two uint loads were legal while uint2/ulong loads were not generally
aligned.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6069",
		Doc:  "nested Metal quant byte loop has exactly two scalar-word loads as its maximum proved packed width",
		Run:  runPS6069,
	},
})

var (
	ps6069BytePointer = regexp.MustCompile(`(?i)\bdevice\s+const\s+(?:u?char|uint8_t|int8_t)\s*\*\s*([A-Za-z_]\w*)`)
	ps6069Assignment  = regexp.MustCompile(`(?m)\b([A-Za-z_]\w*)\s*=\s*([^,;=\n]+)`)
	ps6069ForHeader   = regexp.MustCompile(`\bfor\s*\(\s*(?:(?:const\s+)?(?:int|uint|short|ushort|long|ulong|size_t|int32_t|uint32_t)\s+)?([A-Za-z_]\w*)\s*=\s*([^;]+);\s*([A-Za-z_]\w*)\s*<\s*([^;]+);[^)]*\)`)
	ps6069ForBrace    = regexp.MustCompile(`(?s)\bfor\s*\([^{}]*\)\s*$`)
	ps6069IntegerType = regexp.MustCompile(`(?i)^(?:const)?(?:u?char|u?short|u?int|u?long|size_t|int(?:8|16|32|64)_t|uint(?:8|16|32|64)_t)$`)
)

type ps6069Finding struct {
	kernel string
	source string
	index  string
	offset int
}

func runPS6069(pass *analysis.Pass) (any, error) {
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
			for _, finding := range ps6069PackedWordCandidates(source) {
				pass.Report(analysis.Diagnostic{Pos: literal.Pos(), End: literal.End(), Message: ps6069Message(finding)})
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
		findings := ps6069PackedWordCandidates(string(source))
		if len(findings) == 0 {
			continue
		}
		file := pass.Fset.AddFile(filename, -1, len(source))
		file.SetLinesForContent(source)
		for _, finding := range findings {
			offset := min(max(finding.offset, 0), len(source))
			pass.Reportf(file.Pos(offset), "%s", ps6069Message(finding))
		}
	}
	return nil, nil
}

func ps6069PackedWordCandidates(source string) []ps6069Finding {
	if !strings.Contains(source, "device") || !strings.Contains(source, "for") {
		return nil
	}
	findings := ps6069DirectPackedWordCandidates(source)
	for _, fragment := range ps6068ConcatenatedCStringFragments(source) {
		for _, finding := range ps6069DirectPackedWordCandidates(fragment.source) {
			finding.offset = fragment.offset
			findings = append(findings, finding)
		}
	}
	return findings
}

func ps6069DirectPackedWordCandidates(source string) []ps6069Finding {
	clean := ps6053BlankCommentsAndStrings(source)
	if !ps6053LooksGPU(clean) {
		return nil
	}
	var findings []ps6069Finding
	for _, kernel := range ps6053Kernels(clean) {
		if !ps6069QuantContext(kernel.name) || ps6069Validated(source, kernel) {
			continue
		}
		body := clean[kernel.start:kernel.end]
		bytePointerMatches := ps6069BytePointer.FindAllStringSubmatch(body, -1)
		byteSources := make(map[string]bool, len(bytePointerMatches))
		for _, match := range bytePointerMatches {
			byteSources[match[1]] = true
		}
		if len(byteSources) == 0 {
			continue
		}
		for _, match := range ps6069ForHeader.FindAllStringSubmatchIndex(body, -1) {
			loopVariable := body[match[2]:match[3]]
			lower := body[match[4]:match[5]]
			conditionVariable := body[match[6]:match[7]]
			upper := body[match[8]:match[9]]
			if loopVariable != conditionVariable || ps6069LoopSpan(lower, upper) != 8 || !ps6069InsideLoop(body, match[0]) {
				continue
			}
			bodyStart := match[1]
			for bodyStart < len(body) && (body[bodyStart] == ' ' || body[bodyStart] == '\t' || body[bodyStart] == '\r' || body[bodyStart] == '\n') {
				bodyStart++
			}
			if bodyStart >= len(body) || body[bodyStart] != '{' {
				continue
			}
			bodyEnd := ps6053MatchingBrace(body, bodyStart)
			if bodyEnd < 0 {
				continue
			}
			assignments := ps6069Assignments(body[:match[0]])
			for byteSource := range byteSources {
				load := regexp.MustCompile(`\b` + regexp.QuoteMeta(byteSource) + `\s*\[\s*([^\]]+)\]`)
				loads := load.FindAllStringSubmatchIndex(body[bodyStart+1:bodyEnd], -1)
				if len(loads) != 1 {
					continue
				}
				index := body[bodyStart+1+loads[0][2] : bodyStart+1+loads[0][3]]
				if !ps6068ContainsWord(index, loopVariable) {
					continue
				}
				startIndex := ps6069ReplaceWord(index, loopVariable, "("+lower+")")
				alignment := ps6069Alignment(startIndex, assignments, map[string]bool{})
				if alignment != 2 {
					continue
				}
				findings = append(findings, ps6069Finding{
					kernel: kernel.name,
					source: byteSource,
					index:  strings.TrimSpace(startIndex),
					offset: kernel.start + match[0],
				})
			}
		}
	}
	return findings
}

func ps6069QuantContext(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "quant", "dequant", "unpack", "decode", "qmatmul", "q2k", "q3k", "q4k", "q5k", "q6k")
}

func ps6069LoopSpan(lower, upper string) int64 {
	lower = ps6069CompactExpression(lower)
	upper = ps6069CompactExpression(upper)
	for _, candidate := range []struct {
		prefix string
		suffix string
	}{
		{prefix: lower + "+"},
		{suffix: "+" + lower},
	} {
		var delta string
		switch {
		case candidate.prefix != "" && strings.HasPrefix(upper, candidate.prefix):
			delta = strings.TrimPrefix(upper, candidate.prefix)
		case candidate.suffix != "" && strings.HasSuffix(upper, candidate.suffix):
			delta = strings.TrimSuffix(upper, candidate.suffix)
		}
		if delta != "" {
			if value, ok := ps6069Integer(delta); ok {
				return value
			}
		}
	}
	return 0
}

func ps6069InsideLoop(body string, offset int) bool {
	stack := make([]bool, 0, 8)
	for index := 0; index < offset; index++ {
		switch body[index] {
		case '{':
			start := max(strings.LastIndexAny(body[:index], "{}")+1, 0)
			stack = append(stack, ps6069ForBrace.MatchString(strings.TrimSpace(body[start:index])))
		case '}':
			if len(stack) != 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return slices.Contains(stack, true)
}

func ps6069Assignments(source string) map[string]string {
	matches := ps6069Assignment.FindAllStringSubmatch(source, -1)
	assignments := make(map[string]string, len(matches))
	for _, match := range matches {
		assignments[match[1]] = strings.TrimSpace(match[2])
	}
	return assignments
}

// ps6069Alignment returns the number of trailing zero bits guaranteed by an
// expression for every runtime value of unknown terms. Zero is represented by
// a large cap because it is divisible by every relevant power of two.
func ps6069Alignment(expression string, assignments map[string]string, seen map[string]bool) int {
	expression = ps6069StripExpression(expression)
	if expression == "" {
		return 0
	}
	if terms := ps6069SplitTopLevel(expression, "+-"); len(terms) > 1 {
		alignment := 63
		for _, term := range terms {
			alignment = min(alignment, ps6069Alignment(term, assignments, seen))
		}
		return alignment
	}
	if factors := ps6069SplitTopLevel(expression, "*"); len(factors) > 1 {
		alignment := 0
		for _, factor := range factors {
			alignment = min(63, alignment+ps6069Alignment(factor, assignments, seen))
		}
		return alignment
	}
	if value, ok := ps6069Integer(expression); ok {
		if value == 0 {
			return 63
		}
		return bits.TrailingZeros64(uint64(value))
	}
	if ps6069Identifier(expression) {
		if seen[expression] {
			return 0
		}
		initializer, ok := assignments[expression]
		if !ok {
			return 0
		}
		seen[expression] = true
		alignment := ps6069Alignment(initializer, assignments, seen)
		delete(seen, expression)
		return alignment
	}
	return 0
}

func ps6069StripExpression(expression string) string {
	expression = strings.TrimSpace(expression)
	for expression != "" {
		if expression[0] == '+' || expression[0] == '-' {
			if expression[0] == '+' {
				expression = strings.TrimSpace(expression[1:])
				continue
			}
			return "-1*(" + strings.TrimSpace(expression[1:]) + ")"
		}
		if expression[0] != '(' {
			break
		}
		close := ps6069MatchingParen(expression, 0)
		if close < 0 {
			break
		}
		inside := ps6007NormalizeName(expression[1:close])
		switch {
		case close == len(expression)-1:
			expression = strings.TrimSpace(expression[1:close])
		case ps6069IntegerType.MatchString(inside):
			expression = strings.TrimSpace(expression[close+1:])
		default:
			return expression
		}
	}
	return expression
}

func ps6069MatchingParen(expression string, open int) int {
	depth := 0
	for index := open; index < len(expression); index++ {
		switch expression[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func ps6069SplitTopLevel(expression, operators string) []string {
	var parts []string
	depth := 0
	start := 0
	for index := 0; index < len(expression); index++ {
		switch expression[index] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		default:
			if depth == 0 && index > start && strings.ContainsRune(operators, rune(expression[index])) {
				parts = append(parts, expression[start:index])
				start = index + 1
			}
		}
	}
	if start != 0 {
		parts = append(parts, expression[start:])
	}
	return parts
}

func ps6069Integer(expression string) (int64, bool) {
	expression = strings.TrimRight(strings.TrimSpace(expression), "uUlL")
	value, err := strconv.ParseInt(expression, 0, 64)
	return value, err == nil
}

func ps6069Identifier(expression string) bool {
	if expression == "" || !(expression[0] == '_' || expression[0] >= 'A' && expression[0] <= 'Z' || expression[0] >= 'a' && expression[0] <= 'z') {
		return false
	}
	for index := 1; index < len(expression); index++ {
		character := expression[index]
		if character != '_' && !(character >= 'A' && character <= 'Z') && !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func ps6069CompactExpression(expression string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '(' || r == ')' {
			return -1
		}
		return r
	}, expression)
}

func ps6069ReplaceWord(source, old, replacement string) string {
	return regexp.MustCompile(`\b`+regexp.QuoteMeta(old)+`\b`).ReplaceAllString(source, replacement)
}

func ps6069Validated(source string, kernel ps6053Kernel) bool {
	start := max(kernel.start-256, 0)
	text := ps6058Compact(source[start:kernel.end])
	return strings.Contains(text, "perfscanpackedquantwordloadvalidated") ||
		strings.Contains(text, "perfscanpackedquantloadwidthvalidated")
}

func ps6069Message(finding ps6069Finding) string {
	return "Metal packed-quant kernel " + finding.kernel + " reads an eight-byte " + finding.source + " span in a nested byte loop whose full start offset " + finding.index + " is provably 4-byte aligned but not provably 8-byte aligned; benchmark exactly two scalar uint loads plus register extraction, not uint2/ulong or wider device loads, after proving actual base/row/block/plane/group/lane alignment — keep the candidate route-gated/default-off, preserve byte order and accumulation/parity, and retain the historical pipeline below a same-binary measured crossover (advisory, no automatic fix)"
}
