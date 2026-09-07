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

// PS6104 implements owner issue #896. It finds row-boundary predicates inside
// accelerator block loops when the predicate is invariant across that loop.
var PS6104 = register(&lint.Check{
	ID:       "PS6104",
	Category: "verify",
	Slug:     "accelerator-loop-invariant-tail-guard",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an accelerator block loop repeats a launch-invariant row-tail predicate",
		Text: `Accelerator kernels that map several output rows to one SIMD group
often retain a row-validity predicate inside every reduction or quant block.
For launch shapes that make every mapped row valid, a compile-time function
constant can remove the repeated inner predicate while the guarded kernel
remains the odd/tail fallback.

PS6104 scans embedded Go strings, concatenated C/Objective-C shader strings,
and package-owned Metal, CUDA/HIP, Vulkan/GLSL, and OpenCL sources. It requires
a zero-based unit-step block/reduction loop and an in-loop if predicate with an
explicit row-like boundary comparison. The predicate and its simple local
assignment chain must not depend on the loop index, a call, a channel-like
native operation, volatile/atomic state, or a value declared from the loop
index. Conditions already controlled by a Metal function_constant remain
silent, as do one-trip loops, non-block loops, nested-loop predicates assigned
to the wrong loop, comments, quoted examples, and kernels carrying a
//perfscan:invariant-tail-guard-validated annotation.

The finding is ranked evidence, not a rewrite. Source invariance does not prove
that the compiler leaves the branch in native code, that specialization is
free, or that the affected invocation share can clear a production gate. Keep
one separately selectable specialized pipeline and the guarded fallback;
record the exact even/aligned shape precondition, inspect generated native code
or counters, alternate control/candidate in one warmed binary across the full
shape matrix, weight leaf deltas by production invocation share, and reject a
candidate whose complete stream misses its predeclared promotion floor.`,
		Before: `for (uint block = 0; block < blocks; ++block) {
	if (row < outputRows) {
		accumulate_block(row, block);
	}
}
if (row < outputRows) output[row] = result;`,
		After: `// Function-constant route only when every mapped row is valid.
for (uint block = 0; block < blocks; ++block) {
	accumulate_block(row, block);
}
// Preserve this guard and the original guarded pipeline for tails.
if (row < outputRows) output[row] = result;`,
		MeasuredWin: `Owner issue #896 measured an Apple-M2-Pro Metal Q4_K/Q6_K
two-row matvec specialization. Leaf results ranged from 0.9247x to 1.0608x;
the longer winning Q4_K K2048xN5632 cell reached 1.0462x. After routing only
that narrow shape, seven alternating complete 22-layer TinyLlama projection
streams produced a 1.0112x median, below the frozen 1.03x promotion gate, so
the bit-identical candidate was reverted.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6104",
		Doc:  "accelerator block loop repeats a launch-invariant row-tail predicate",
		Run:  runPS6104,
	},
})

var (
	ps6104If             = regexp.MustCompile(`\bif\s*\(`)
	ps6104Identifier     = regexp.MustCompile(`\b[A-Za-z_]\w*\b`)
	ps6104Comparison     = regexp.MustCompile(`(?:<=|>=|<|>)`)
	ps6104FunctionConst  = regexp.MustCompile(`(?s)\bconstant\s+[A-Za-z_]\w*\s+([A-Za-z_]\w*)\s*\[\[\s*function_constant\s*\(`)
	ps6104Call           = regexp.MustCompile(`\b([A-Za-z_]\w*)\s*\(`)
	ps6104AssignmentDecl = regexp.MustCompile(`(?m)\b([A-Za-z_]\w*)\s*=\s*([^;\n]+)`)
	ps6104ForHeader      = regexp.MustCompile(`\bfor\s*\(\s*(?:(?:const\s+)?(?:uint|int|uint32_t|int32_t|size_t|ushort|long)\s+)?([A-Za-z_]\w*)\s*=\s*0(?:[uUlL]*)\s*;\s*([A-Za-z_]\w*)\s*<\s*([^;]+?)\s*;\s*([^)]+)\)`)
)

type ps6104Finding struct {
	kernel    string
	loopIndex string
	loopBound string
	predicate string
	offset    int
}

type ps6104Condition struct {
	expression string
	offset     int
}

type ps6104Assignment struct {
	expression string
	offset     int
}

func runPS6104(pass *analysis.Pass) (any, error) {
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
			for _, finding := range ps6104InvariantTailGuards(source) {
				pass.Report(analysis.Diagnostic{Pos: literal.Pos(), End: literal.End(), Message: ps6104Message(finding)})
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
		findings := ps6104InvariantTailGuards(string(source))
		if len(findings) == 0 {
			continue
		}
		file := pass.Fset.AddFile(filename, -1, len(source))
		file.SetLinesForContent(source)
		for _, finding := range findings {
			offset := min(max(finding.offset, 0), len(source))
			pass.Reportf(file.Pos(offset), "%s", ps6104Message(finding))
		}
	}
	return nil, nil
}

func ps6104InvariantTailGuards(source string) []ps6104Finding {
	if !ps6053LooksGPU(source) {
		return nil
	}
	findings := ps6104DirectInvariantTailGuards(source)
	for _, fragment := range ps6068ConcatenatedCStringFragments(source) {
		for _, finding := range ps6104DirectInvariantTailGuards(fragment.source) {
			finding.offset = fragment.offset
			findings = append(findings, finding)
		}
	}
	return findings
}

func ps6104DirectInvariantTailGuards(source string) []ps6104Finding {
	clean := ps6053BlankCommentsAndStrings(source)
	if !ps6053LooksGPU(clean) {
		return nil
	}
	var findings []ps6104Finding
	functionConstants := ps6104FunctionConstants(clean)
	for _, kernel := range ps6053Kernels(clean) {
		if ps6104Validated(source, kernel) {
			continue
		}
		body := clean[kernel.start:kernel.end]
		loops := ps6104Loops(body)
		loopIndexes := make(map[string]bool, len(loops))
		for _, loop := range loops {
			loopIndexes[loop.index] = true
		}
		for _, loop := range loops {
			if !ps6104RepeatedBlockLoop(loop) {
				continue
			}
			assignments := ps6104Assignments(body, kernel.start)
			mutations, directCallRoots := ps6104MutationFacts(loop.body)
			for _, condition := range ps6104Conditions(loop.body) {
				loopBodyStart := loop.end - len(loop.body)
				conditionOffset := kernel.start + loopBodyStart + condition.offset
				loopStart := kernel.start + loop.start
				loopEnd := kernel.start + loop.end
				if ps6104InsideNestedLoop(loop.body, condition.offset) || !ps6104TailPredicate(condition.expression, assignments, conditionOffset, loopStart, loopEnd, loop.index, loopIndexes, functionConstants, mutations, directCallRoots) {
					continue
				}
				findings = append(findings, ps6104Finding{
					kernel: kernel.name, loopIndex: loop.index, loopBound: loop.bound,
					predicate: strings.Join(strings.Fields(condition.expression), " "),
					offset:    conditionOffset,
				})
				break
			}
			if len(findings) != 0 && findings[len(findings)-1].kernel == kernel.name {
				break
			}
		}
	}
	return findings
}

func ps6104Loops(source string) []ps6053Loop {
	var loops []ps6053Loop
	seen := make([]bool, len(source)+1)
	matches := ps6104ForHeader.FindAllStringSubmatchIndex(source, -1)
	loops = make([]ps6053Loop, 0, len(matches))
	for _, match := range matches {
		index := source[match[2]:match[3]]
		if index != source[match[4]:match[5]] || !ps6053UnitStep(index, source[match[8]:match[9]]) {
			continue
		}
		bodyStart := match[1]
		for bodyStart < len(source) && strings.ContainsRune(" \t\r\n", rune(source[bodyStart])) {
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
		seen[match[0]] = true
		loops = append(loops, ps6053Loop{index: index, bound: strings.TrimSpace(source[match[6]:match[7]]), start: match[0], end: bodyEnd, body: source[bodyStart:bodyEnd]})
	}
	for _, loop := range ps6053Loops(source) {
		if !seen[loop.start] {
			loops = append(loops, loop)
		}
	}
	slices.SortFunc(loops, func(left, right ps6053Loop) int { return left.start - right.start })
	return loops
}

func ps6104RepeatedBlockLoop(loop ps6053Loop) bool {
	plainBound := strings.TrimSpace(strings.TrimRight(loop.bound, "uUlL"))
	if count, err := strconv.Atoi(plainBound); err == nil && count < 2 {
		return false
	}
	name := ps6007NormalizeName(loop.index + loop.bound)
	index := ps6007NormalizeName(loop.index)
	if index == "k" || index == "ib" || index == "kb" || ps6007ContainsAny(name, "block", "tile", "chunk", "super", "quant", "group", "nb", "nk") {
		return true
	}
	for _, identifier := range ps6104Identifier.FindAllString(loop.bound, -1) {
		if strings.EqualFold(identifier, "k") || strings.EqualFold(identifier, "blocks") || strings.EqualFold(identifier, "tiles") {
			return true
		}
	}
	return false
}

func ps6104Conditions(body string) []ps6104Condition {
	var conditions []ps6104Condition
	for _, match := range ps6104If.FindAllStringIndex(body, -1) {
		open := strings.IndexByte(body[match[0]:match[1]], '(') + match[0]
		close := ps6069MatchingParen(body, open)
		if open < match[0] || close < 0 {
			continue
		}
		conditions = append(conditions, ps6104Condition{expression: body[open+1 : close], offset: match[0]})
	}
	return conditions
}

func ps6104InsideNestedLoop(body string, offset int) bool {
	for _, nested := range ps6104Loops(body) {
		if nested.start < offset && offset < nested.end {
			return true
		}
	}
	return false
}

func ps6104MutationFacts(body string) (map[string]bool, map[string]bool) {
	roots := ps6104NativeMutationRoots(body)
	exposedRoots, directCallRoots := ps6104NativeCallRoots(body)
	result := make(map[string]bool, len(roots)+len(exposedRoots))
	for _, root := range roots {
		// Mutating an indexed element, field, or dereference can change a
		// predicate that reads through the same aggregate root. Treat the
		// whole root as loop-variant instead of pretending only direct
		// identifiers write.
		result[root] = true
	}
	for root := range exposedRoots {
		// A call can mutate a native lvalue whose address is exposed even
		// when the loop contains no source-visible assignment to that root.
		result[root] = true
	}
	return result, directCallRoots
}

func ps6104NativeMutationRoots(source string) []string {
	tokens := ps6104NativeTokens(source)
	seen := make(map[string]bool)
	var roots []string
	add := func(root string) {
		if root != "" && !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}
	for index, token := range tokens {
		switch token {
		case "++", "--":
			if root, ok := ps6104NativeLValueBefore(tokens, index); ok {
				add(root)
			}
			if _, root, ok := ps6104ParseNativeLValue(tokens, index+1); ok {
				add(root)
			}
		case "=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
			if root, ok := ps6104NativeLValueBefore(tokens, index); ok {
				add(root)
			}
		}
	}
	return roots
}

func ps6104NativeCallRoots(source string) (map[string]bool, map[string]bool) {
	tokens := ps6104NativeTokens(source)
	exposed := make(map[string]bool)
	direct := make(map[string]bool)
	for open := 1; open < len(tokens); open++ {
		if tokens[open] != "(" || !ps6104NativeCallName(tokens[open-1]) {
			continue
		}
		close := ps6104MatchingNativeToken(tokens, open, "(", ")")
		if close < 0 {
			continue
		}
		for _, argument := range ps6104NativeArgumentRanges(tokens, open+1, close) {
			start, end := argument[0], argument[1]
			if root, ok := ps6104NativeDirectRoot(tokens, start, end); ok {
				direct[root] = true
			}
			for index := start; index < end; index++ {
				if tokens[index] != "&" || !ps6104NativeUnaryOperator(tokens, index, start) {
					continue
				}
				next, root, ok := ps6104ParseNativeLValue(tokens, index+1)
				if ok && next <= end {
					exposed[root] = true
				}
			}
		}
	}
	return exposed, direct
}

func ps6104NativeCallName(token string) bool {
	if !ps6104NativeIdentifier(token) || ps6104TypeCast(token) {
		return false
	}
	switch token {
	case "if", "for", "while", "switch", "sizeof", "alignof", "return":
		return false
	}
	return true
}

func ps6104NativeArgumentRanges(tokens []string, start, end int) [][2]int {
	if start >= end {
		return nil
	}
	var result [][2]int
	argumentStart := start
	parentheses, brackets, braces := 0, 0, 0
	for index := start; index < end; index++ {
		switch tokens[index] {
		case "(":
			parentheses++
		case ")":
			parentheses--
		case "[":
			brackets++
		case "]":
			brackets--
		case "{":
			braces++
		case "}":
			braces--
		case ",":
			if parentheses == 0 && brackets == 0 && braces == 0 {
				result = append(result, [2]int{argumentStart, index})
				argumentStart = index + 1
			}
		}
	}
	return append(result, [2]int{argumentStart, end})
}

func ps6104NativeDirectRoot(tokens []string, start, end int) (string, bool) {
	next, root, ok := ps6104ParseNativeLValue(tokens, start)
	if !ok || next != end {
		return "", false
	}
	return root, true
}

func ps6104NativeUnaryOperator(tokens []string, index, expressionStart int) bool {
	if index == expressionStart {
		return true
	}
	switch tokens[index-1] {
	case "(", "[", "{", ",", "?", ":", "=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", "+", "-", "*", "/", "%", "!", "~", "&&", "||", "==", "!=", "<", ">", "<=", ">=":
		return true
	}
	return false
}

func ps6104NativeLValueBefore(tokens []string, end int) (string, bool) {
	start := 0
	parentheses := 0
	brackets := 0

scan:
	for index := end - 1; index >= 0; index-- {
		switch tokens[index] {
		case ")":
			parentheses++
			continue
		case "(":
			if parentheses > 0 {
				parentheses--
				continue
			}
		case "]":
			brackets++
			continue
		case "[":
			if brackets > 0 {
				brackets--
				continue
			}
		}
		if parentheses != 0 || brackets != 0 {
			continue
		}
		switch tokens[index] {
		case ";", "{", "}", ",", "?", ":", "=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
			start = index + 1
			break scan
		}
	}
	for index := start; index < end; index++ {
		next, root, ok := ps6104ParseNativeLValue(tokens, index)
		if ok && next == end {
			return root, true
		}
	}
	return "", false
}

func ps6104ParseNativeLValue(tokens []string, start int) (int, string, bool) {
	index := start
	indirect := false
	for index < len(tokens) && tokens[index] == "*" {
		indirect = true
		index++
	}
	if index >= len(tokens) {
		return start, "", false
	}
	var root string
	switch {
	case ps6104NativeIdentifier(tokens[index]):
		root = tokens[index]
		index++
	case tokens[index] == "(":
		next, innerRoot, ok := ps6104ParseNativeLValue(tokens, index+1)
		if !ok || next >= len(tokens) || tokens[next] != ")" {
			return start, "", false
		}
		root = innerRoot
		index = next + 1
	default:
		return start, "", false
	}
	for index < len(tokens) {
		switch tokens[index] {
		case "[":
			close := ps6104MatchingNativeToken(tokens, index, "[", "]")
			if close < 0 {
				return start, "", false
			}
			indirect = true
			index = close + 1
		case ".", "->":
			if index+1 >= len(tokens) || !ps6104NativeIdentifier(tokens[index+1]) {
				return start, "", false
			}
			indirect = true
			index += 2
		default:
			return index, root, root != "" && (indirect || index > start)
		}
	}
	return index, root, root != "" && (indirect || index > start)
}

func ps6104MatchingNativeToken(tokens []string, start int, open, close string) int {
	depth := 0
	for index := start; index < len(tokens); index++ {
		switch tokens[index] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func ps6104NativeTokens(source string) []string {
	var tokens []string
	for index := 0; index < len(source); {
		if strings.ContainsRune(" \t\r\n", rune(source[index])) {
			index++
			continue
		}
		if ps6104NativeIdentifierStart(source[index]) {
			end := index + 1
			for end < len(source) && ps6104NativeIdentifierContinue(source[end]) {
				end++
			}
			tokens = append(tokens, source[index:end])
			index = end
			continue
		}
		matched := false
		for _, operator := range []string{"<<=", ">>=", "++", "--", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "->", "==", "!=", "<=", ">=", "&&", "||"} {
			if strings.HasPrefix(source[index:], operator) {
				tokens = append(tokens, operator)
				index += len(operator)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		tokens = append(tokens, source[index:index+1])
		index++
	}
	return tokens
}

func ps6104NativeIdentifier(token string) bool {
	if token == "" || !ps6104NativeIdentifierStart(token[0]) {
		return false
	}
	for index := 1; index < len(token); index++ {
		if !ps6104NativeIdentifierContinue(token[index]) {
			return false
		}
	}
	return true
}

func ps6104NativeIdentifierStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func ps6104NativeIdentifierContinue(value byte) bool {
	return ps6104NativeIdentifierStart(value) || value >= '0' && value <= '9'
}

func ps6104Assignments(body string, base int) map[string][]ps6104Assignment {
	matches := ps6104AssignmentDecl.FindAllStringSubmatchIndex(body, -1)
	result := make(map[string][]ps6104Assignment, len(matches))
	for _, match := range matches {
		name := body[match[2]:match[3]]
		expression := body[match[4]:match[5]]
		if strings.HasPrefix(strings.TrimSpace(expression), "=") {
			continue
		}
		result[name] = append(result[name], ps6104Assignment{expression: expression, offset: base + match[0]})
	}
	return result
}

func ps6104FunctionConstants(body string) map[string]bool {
	matches := ps6104FunctionConst.FindAllStringSubmatch(body, -1)
	result := make(map[string]bool, len(matches))
	for _, match := range matches {
		result[match[1]] = true
	}
	return result
}

func ps6104TailPredicate(expression string, assignments map[string][]ps6104Assignment, before, loopStart, loopEnd int, loopIndex string, loopIndexes, functionConstants, mutations, directCallRoots map[string]bool) bool {
	resolvedAliases := make(map[string]bool)
	expression, before = ps6104ResolvePredicate(expression, assignments, before, resolvedAliases)
	if !ps6104Comparison.MatchString(expression) || ps6007ContainsAny(strings.ToLower(expression), "atomic", "volatile", "barrier") {
		return false
	}
	for _, match := range ps6104Call.FindAllStringSubmatch(expression, -1) {
		if !ps6104TypeCast(match[1]) {
			return false
		}
	}
	active := make(map[string]bool)
	forbidden := make(map[string]bool, len(loopIndexes)+len(functionConstants)+len(mutations))
	for name := range loopIndexes {
		forbidden[name] = true
	}
	for name := range functionConstants {
		forbidden[name] = true
	}
	for name := range mutations {
		forbidden[name] = true
	}
	for name := range resolvedAliases {
		if forbidden[name] {
			return false
		}
	}
	for name := range ps6104NativeIndirectRoots(expression) {
		if directCallRoots[name] {
			return false
		}
	}
	if ps6104DependsOn(expression, assignments, before, loopStart, loopEnd, loopIndex, forbidden, active) {
		return false
	}
	return ps6104RowSignal(expression, assignments, before, make(map[string]bool)) &&
		ps6104BoundarySignal(expression, assignments, before, make(map[string]bool))
}

func ps6104NativeIndirectRoots(expression string) map[string]bool {
	tokens := ps6104NativeTokens(expression)
	result := make(map[string]bool)
	for start := range tokens {
		if tokens[start] == "*" && !ps6104NativeUnaryOperator(tokens, start, 0) {
			continue
		}
		next, root, ok := ps6104ParseNativeLValue(tokens, start)
		if !ok || next <= start {
			continue
		}
		for _, token := range tokens[start:next] {
			switch token {
			case "*", "[", ".", "->":
				result[root] = true
			}
		}
	}
	return result
}

func ps6104ResolvePredicate(expression string, assignments map[string][]ps6104Assignment, before int, active map[string]bool) (string, int) {
	trimmed := strings.TrimSpace(expression)
	for strings.HasPrefix(trimmed, "(") && ps6069MatchingParen(trimmed, 0) == len(trimmed)-1 {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	if !ps6104Identifier.MatchString(trimmed) || ps6104Identifier.FindString(trimmed) != trimmed || active[trimmed] {
		return expression, before
	}
	assignment, ok := ps6104LastAssignment(assignments[trimmed], before)
	if !ok {
		return expression, before
	}
	active[trimmed] = true
	return ps6104ResolvePredicate(assignment.expression, assignments, assignment.offset, active)
}

func ps6104DependsOn(expression string, assignments map[string][]ps6104Assignment, before, loopStart, loopEnd int, target string, forbidden map[string]bool, active map[string]bool) bool {
	for _, identifier := range ps6104Identifier.FindAllString(expression, -1) {
		if identifier == target || forbidden[identifier] {
			return true
		}
		if active[identifier] {
			return true
		}
		assignment, ok := ps6104LastAssignment(assignments[identifier], before)
		if !ok {
			continue
		}
		if assignment.offset >= loopStart && assignment.offset <= loopEnd {
			return true
		}
		active[identifier] = true
		if ps6104DependsOn(assignment.expression, assignments, assignment.offset, loopStart, loopEnd, target, forbidden, active) {
			return true
		}
		delete(active, identifier)
	}
	return false
}

func ps6104LastAssignment(assignments []ps6104Assignment, before int) (ps6104Assignment, bool) {
	for index := len(assignments) - 1; index >= 0; index-- {
		if assignments[index].offset < before {
			return assignments[index], true
		}
	}
	return ps6104Assignment{}, false
}

func ps6104RowSignal(expression string, assignments map[string][]ps6104Assignment, before int, active map[string]bool) bool {
	for _, identifier := range ps6104Identifier.FindAllString(expression, -1) {
		normalized := ps6007NormalizeName(identifier)
		if ps6007ContainsAny(normalized, "row", "outrow", "dstrow") {
			return true
		}
		if active[identifier] {
			continue
		}
		assignment, ok := ps6104LastAssignment(assignments[identifier], before)
		if !ok {
			continue
		}
		active[identifier] = true
		if ps6104RowSignal(assignment.expression, assignments, assignment.offset, active) {
			return true
		}
		delete(active, identifier)
	}
	return false
}

func ps6104BoundarySignal(expression string, assignments map[string][]ps6104Assignment, before int, active map[string]bool) bool {
	for _, identifier := range ps6104Identifier.FindAllString(expression, -1) {
		normalized := ps6007NormalizeName(identifier)
		if identifier == "N" || ps6007ContainsAny(normalized, "nrows", "rowcount", "outputrows", "height", "rows", "size", "dimension") {
			return true
		}
		if active[identifier] {
			continue
		}
		assignment, ok := ps6104LastAssignment(assignments[identifier], before)
		if !ok {
			continue
		}
		active[identifier] = true
		if ps6104BoundarySignal(assignment.expression, assignments, assignment.offset, active) {
			return true
		}
		delete(active, identifier)
	}
	return false
}

func ps6104TypeCast(name string) bool {
	switch strings.ToLower(name) {
	case "bool", "char", "uchar", "short", "ushort", "int", "uint", "long", "ulong", "size_t", "int32_t", "uint32_t", "int64_t", "uint64_t":
		return true
	}
	return false
}

func ps6104Validated(source string, kernel ps6053Kernel) bool {
	start := max(kernel.start-256, 0)
	text := ps6058Compact(source[start:kernel.end])
	return strings.Contains(text, "perfscaninvarianttailguardvalidated") ||
		strings.Contains(text, "perfscanacceleratortailguardvalidated")
}

func ps6104Message(finding ps6104Finding) string {
	return "GPU kernel " + finding.kernel + " repeats launch-invariant row-tail predicate `" + finding.predicate + "` inside every " + finding.loopIndex + "<" + finding.loopBound + " block iteration; evaluate a separately selectable compile-time-specialized route only for the exact all-rows-valid shape and retain the guarded tail pipeline/final store; first confirm the compiler has not already removed or predicated the branch in native code, then use warmed same-binary AB/BA leaf measurements and invocation-weighted production-shape gates—the owner leaf reached 1.0462x but diluted to 1.0112x, below its 1.03x stream gate (advisory, no automatic fix)"
}
