package checks

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6078 implements owner issue #795. It traces architecture-partitioned
// boolean capability values through constant-foldable guards and the reverse
// same-package call graph to exported operations.
var PS6078 = register(&lint.Check{
	ID:       "PS6078",
	Category: "verify",
	Slug:     "architecture-capability-disabled-callgraph",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an architecture capability flag constant-folds optimized branches out of public operations",
		Text: `A missing architecture optimization is not always visible at the leaf.
An architecture-specific false constant can guard a fast path several frames
above it, so one disabled capability silently routes multiple exported
operations through scalar code even when a related vector primitive exists.

This check implements owner issue #795. It parses active and ignored package
files, compares same-named package const/var declarations initialized by true
or false across satisfiable, mutually exclusive GOOS/GOARCH/feature
partitions, and analyzes each false partition. The shared partition solver
combines filename constraints with //go:build expressions, including explicit
experiment tags.

For every differing flag, the check evaluates direct boolean guards under the
false value. It supports the flag, !flag, ==/!= true/false, and &&/|| forms
whose result becomes constant. The disabled branch must call a fast/optimized/
SIMD/NEON/AVX/SSE/assembly/native/kernel-named leaf or a function independently
classified as assembly/vector-backed. It also recognizes the common early
fallback form ` + "`if !flag { scalar(); return }; fast()`" + ` by treating
following statements as disabled when the constant-true branch terminates.

The analyzer then builds the false-partition same-package reverse call graph,
reports every directly gated function and disabled leaf, and traces them to
reachable exported operations. Exact false/true constraint labels and source
locations are printed and attached as related spans. Priority increases when
the disabled partition contains another enabled same-dtype capability or an
assembly/SIMD/vector-width leaf, because that is local evidence that the
architecture already has related vector infrastructure.

Only simple package boolean initializers are compared. Overlapping or
unsatisfiable partitions, equal values, shadowed flag names, guards without an
optimized disabled branch, indirect function values, nested closures, and
//perfscan:architecture-capability-validated declarations/functions stay
silent.

There is NO automatic fix. Turning on a capability changes reachable code and
may expose unvalidated alignment, length, aliasing, tail, approximation,
special-value, or initialization behavior across several public operations.
Implement the missing leaf and keep the flag/candidate separately selectable;
validate every reported public route with instruction inspection and
same-binary alternating-order complete-operation campaigns before promotion.`,
		Before: `// capability_arm64.go
const vexpF64Fast = false

func expF64(dst, src []float64) {
	if vexpF64Fast { vexpF64NEON(dst, src); return }
	expF64Scalar(dst, src)
}

func Exp(dst, src []float64) { expF64(dst, src) }`,
		After: `// Implement and validate the missing architecture leaf first.
// Keep the capability separately selectable during routed AB/BA campaigns.
// Leave a broad flag false when unrelated consumers remain unvalidated.`,
		MeasuredWin: `Owner issue #795 was validated by merged goai change #1127
(merge d3d2f68a35addbc2784c7799486a767818fef016) after two complete green
15-check matrices. Four arm64 F64 transcendental leaves gained proven NEON
implementations while the architecture-wide vexpF64Fast flag correctly stayed
false because it also gates unrelated Exp/WKV/SSM routes. Three alternating
paired count=7 M2 Pro campaigns measured 62.69-63.50% less direct-leaf latency
and a 51.50-52.61% six-operation geomean gain, with p=0.001 in every cell.
This check therefore maps exact gated consumers and nearby leaf evidence; it
does not recommend promoting a broad flag from a related leaf alone.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6078",
		Doc:  "architecture capability values disable optimized branches reachable from public operations",
		Run:  runPS6078,
	},
})

type ps6078Capability struct {
	name   string
	value  bool
	pos    token.Pos
	end    token.Pos
	source *ps6077Source
}

type ps6078Function struct {
	name       string
	function   *ast.FuncDecl
	source     *ps6077Source
	vectorKind string
	vectorRank int
}

type ps6078Gate struct {
	function ps6078Function
	leaves   []string
	position token.Pos
}

type ps6078Finding struct {
	disabled            ps6078Capability
	enabled             []ps6078Capability
	gates               []ps6078Gate
	public              []string
	relatedCapabilities []string
	relatedVectors      []string
}

func runPS6078(pass *analysis.Pass) (any, error) {
	sources := ps6077PackageSources(pass)
	capabilities := ps6078Capabilities(sources)
	functions := ps6078Functions(sources)
	byCapability := make(map[string][]ps6078Capability, len(capabilities))
	for _, capability := range capabilities {
		byCapability[capability.name] = append(byCapability[capability.name], capability)
	}
	var findings []ps6078Finding
	for _, variants := range byCapability {
		for _, disabled := range variants {
			if disabled.value {
				continue
			}
			var enabled []ps6078Capability
			for _, candidate := range variants {
				if candidate.value && ps6077MutuallyExclusive(*disabled.source, *candidate.source) {
					enabled = append(enabled, candidate)
				}
			}
			if len(enabled) == 0 {
				continue
			}
			gates, relevantFunctions := ps6078Gates(pass, disabled, functions)
			if len(gates) == 0 {
				continue
			}
			finding := ps6078Finding{
				disabled: disabled, enabled: enabled, gates: gates,
				public: ps6078ReachablePublic(gates, relevantFunctions),
			}
			finding.relatedCapabilities = ps6078RelatedCapabilities(disabled, capabilities)
			finding.relatedVectors = ps6078RelatedVectors(disabled, relevantFunctions)
			findings = append(findings, finding)
		}
	}
	slices.SortFunc(findings, func(left, right ps6078Finding) int {
		leftPriority := len(left.relatedCapabilities) + len(left.relatedVectors)
		rightPriority := len(right.relatedCapabilities) + len(right.relatedVectors)
		if byPriority := cmp.Compare(rightPriority, leftPriority); byPriority != 0 {
			return byPriority
		}
		if bySurface := cmp.Compare(len(right.public), len(left.public)); bySurface != 0 {
			return bySurface
		}
		return cmp.Compare(left.disabled.pos, right.disabled.pos)
	})
	for findingIndex := range findings {
		ps6078Report(pass, &findings[findingIndex])
	}
	return nil, nil
}

func ps6078Capabilities(sources []ps6077Source) []ps6078Capability {
	var result []ps6078Capability
	for sourceIndex := range sources {
		source := &sources[sourceIndex]
		for _, declaration := range source.file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || (general.Tok != token.CONST && general.Tok != token.VAR) || ps6078ValidatedComments(general.Doc) {
				continue
			}
			for _, raw := range general.Specs {
				specification, ok := raw.(*ast.ValueSpec)
				if !ok || ps6078ValidatedComments(specification.Doc) || ps6078ValidatedComments(specification.Comment) ||
					len(specification.Values) != len(specification.Names) {
					continue
				}
				for index, name := range specification.Names {
					value, ok := ps6078BooleanLiteral(specification.Values[index])
					if !ok {
						continue
					}
					result = append(result, ps6078Capability{
						name: name.Name, value: value, pos: name.Pos(), end: name.End(), source: source,
					})
				}
			}
		}
	}
	return result
}

func ps6078BooleanLiteral(expression ast.Expr) (bool, bool) {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return false, false
	}
	switch identifier.Name {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func ps6078Functions(sources []ps6077Source) []ps6078Function {
	var result []ps6078Function
	for sourceIndex := range sources {
		source := &sources[sourceIndex]
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || ps6078ValidatedFunction(function) {
				continue
			}
			kind, rank := ps6077VectorEvidence(function)
			result = append(result, ps6078Function{
				name: function.Name.Name, function: function, source: source,
				vectorKind: kind, vectorRank: rank,
			})
		}
	}
	return result
}

func ps6078Gates(pass *analysis.Pass, disabled ps6078Capability, functions []ps6078Function) ([]ps6078Gate, []ps6078Function) {
	var relevant []ps6078Function
	vectorNames := make(map[string]bool, len(functions))
	declaredNames := make(map[string]bool, len(functions))
	for _, function := range functions {
		if !ps6077PartitionsOverlap(*disabled.source, *function.source) {
			continue
		}
		relevant = append(relevant, function)
		declaredNames[function.name] = true
		if function.vectorRank > 0 {
			vectorNames[function.name] = true
		}
	}
	var gates []ps6078Gate
	for _, function := range relevant {
		if function.function.Body == nil || ps6078Shadows(function.function, disabled.name) {
			continue
		}
		parents := ps6071Parents(function.function.Body)
		ast.Inspect(function.function.Body, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			known, value := ps6078Condition(statement.Cond, disabled.name, disabled.value)
			if !known {
				return true
			}
			var disabledNodes []ast.Node
			if !value {
				disabledNodes = append(disabledNodes, statement.Body)
			} else if statement.Else != nil {
				disabledNodes = append(disabledNodes, statement.Else)
			} else if ps6078Terminates(statement.Body) {
				disabledNodes = append(disabledNodes, ps6078Following(statement, parents)...)
			}
			leaves := ps6078FastCalls(disabledNodes, vectorNames, declaredNames)
			if len(leaves) > 0 {
				gates = append(gates, ps6078Gate{function: function, leaves: leaves, position: statement.If})
			}
			return true
		})
	}
	return gates, relevant
}

func ps6078Condition(expression ast.Expr, flag string, value bool) (bool, bool) {
	expression = ps2110Unparen(expression)
	switch candidate := expression.(type) {
	case *ast.Ident:
		if candidate.Name == flag {
			return true, value
		}
	case *ast.UnaryExpr:
		if candidate.Op == token.NOT {
			if known, result := ps6078Condition(candidate.X, flag, value); known {
				return true, !result
			}
		}
	case *ast.BinaryExpr:
		leftKnown, left := ps6078Condition(candidate.X, flag, value)
		rightKnown, right := ps6078Condition(candidate.Y, flag, value)
		switch candidate.Op {
		case token.LAND:
			if leftKnown && !left || rightKnown && !right {
				return true, false
			}
			if leftKnown && rightKnown {
				return true, left && right
			}
		case token.LOR:
			if leftKnown && left || rightKnown && right {
				return true, true
			}
			if leftKnown && rightKnown {
				return true, left || right
			}
		case token.EQL, token.NEQ:
			if leftKnown {
				if literal, ok := ps6078BooleanLiteral(candidate.Y); ok {
					result := left == literal
					if candidate.Op == token.NEQ {
						result = !result
					}
					return true, result
				}
			}
			if rightKnown {
				if literal, ok := ps6078BooleanLiteral(candidate.X); ok {
					result := right == literal
					if candidate.Op == token.NEQ {
						result = !result
					}
					return true, result
				}
			}
		}
	}
	return false, false
}

func ps6078Terminates(block *ast.BlockStmt) bool {
	if block == nil || len(block.List) == 0 {
		return false
	}
	switch block.List[len(block.List)-1].(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return true
	}
	return false
}

func ps6078Following(statement *ast.IfStmt, parents map[ast.Node]ast.Node) []ast.Node {
	block, ok := parents[statement].(*ast.BlockStmt)
	if !ok {
		return nil
	}
	for index, candidate := range block.List {
		if candidate == statement && index+1 < len(block.List) {
			result := make([]ast.Node, 0, len(block.List)-index-1)
			for _, following := range block.List[index+1:] {
				result = append(result, following)
			}
			return result
		}
	}
	return nil
}

func ps6078FastCalls(nodes []ast.Node, vectorNames, declaredNames map[string]bool) []string {
	seen := make(map[string]bool)
	var result []string
	for _, node := range nodes {
		ast.Inspect(node, func(candidate ast.Node) bool {
			if _, nested := candidate.(*ast.FuncLit); nested {
				return false
			}
			call, ok := candidate.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ps6074CalledName(call.Fun)
			if name != "" && (vectorNames[name] || declaredNames[name] && ps6078OptimizedName(name)) && !seen[name] {
				seen[name] = true
				result = append(result, name)
			}
			return true
		})
	}
	slices.Sort(result)
	return result
}

func ps6078OptimizedName(name string) bool {
	normalized := strings.ToLower(name)
	return ps6007ContainsAny(normalized,
		"fast", "optimized", "vector", "simd", "neon", "avx", "sse", "assembly", "native", "kernel",
	) || strings.HasPrefix(normalized, "asm") || strings.HasSuffix(normalized, "asm")
}

func ps6078Shadows(function *ast.FuncDecl, flag string) bool {
	for _, fields := range []*ast.FieldList{function.Recv, function.Type.Params, function.Type.Results} {
		if fields == nil {
			continue
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				if name.Name == flag {
					return true
				}
			}
		}
	}
	shadowed := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if shadowed {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok == token.DEFINE {
				for _, left := range value.Lhs {
					if identifier, ok := ps2110Unparen(left).(*ast.Ident); ok && identifier.Name == flag {
						shadowed = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, name := range value.Names {
				if name.Name == flag {
					shadowed = true
				}
			}
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{value.Key, value.Value} {
				if identifier, ok := expression.(*ast.Ident); ok && identifier.Name == flag {
					shadowed = true
				}
			}
		}
		return !shadowed
	})
	return shadowed
}

func ps6078ReachablePublic(gates []ps6078Gate, functions []ps6078Function) []string {
	available := make(map[string]bool, len(functions))
	reverse := make(map[string]map[string]bool)
	for _, function := range functions {
		available[function.name] = true
	}
	for _, function := range functions {
		for _, called := range ps6078DirectCalls(function.function.Body) {
			if !available[called] {
				continue
			}
			if reverse[called] == nil {
				reverse[called] = make(map[string]bool)
			}
			reverse[called][function.name] = true
		}
	}
	seen := make(map[string]bool, len(gates))
	var queue []string
	for _, gate := range gates {
		if !seen[gate.function.name] {
			seen[gate.function.name] = true
			queue = append(queue, gate.function.name)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for caller := range reverse[current] {
			if !seen[caller] {
				seen[caller] = true
				queue = append(queue, caller)
			}
		}
	}
	var result []string
	for name := range seen {
		if ast.IsExported(name) {
			result = append(result, name)
		}
	}
	slices.Sort(result)
	return result
}

func ps6078DirectCalls(body *ast.BlockStmt) []string {
	if body == nil {
		return nil
	}
	seen := make(map[string]bool)
	var result []string
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ps6074CalledName(call.Fun)
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
		return true
	})
	return result
}

func ps6078RelatedCapabilities(disabled ps6078Capability, capabilities []ps6078Capability) []string {
	dtype := ps6078DType(disabled.name)
	if dtype == "" {
		return nil
	}
	seen := make(map[string]bool, len(capabilities))
	var result []string
	for _, candidate := range capabilities {
		if candidate.name == disabled.name || !candidate.value || ps6078DType(candidate.name) != dtype ||
			!ps6077PartitionsOverlap(*disabled.source, *candidate.source) || seen[candidate.name] {
			continue
		}
		seen[candidate.name] = true
		result = append(result, candidate.name)
	}
	slices.Sort(result)
	return result
}

func ps6078RelatedVectors(disabled ps6078Capability, functions []ps6078Function) []string {
	dtype := ps6078DType(disabled.name)
	if dtype == "" {
		return nil
	}
	seen := make(map[string]bool, len(functions))
	var result []string
	for _, function := range functions {
		if !ps6078VectorLeaf(function) || ps6078DType(function.name) != dtype || seen[function.name] {
			continue
		}
		seen[function.name] = true
		result = append(result, function.name)
	}
	slices.Sort(result)
	if len(result) > 4 {
		result = result[:4]
	}
	return result
}

func ps6078VectorLeaf(function ps6078Function) bool {
	return function.vectorRank > 0 && (function.function.Body == nil ||
		function.vectorKind == "a multi-lane vector-width loop" || ps6078OptimizedName(function.name))
}

func ps6078DType(name string) string {
	normalized := strings.ToLower(name)
	for _, candidate := range []string{"float64", "f64", "float32", "f32", "bfloat16", "bf16", "float16", "f16"} {
		if strings.Contains(normalized, candidate) {
			return candidate
		}
	}
	return ""
}

func ps6078Report(pass *analysis.Pass, finding *ps6078Finding) {
	disabledPosition := pass.Fset.Position(finding.disabled.pos)
	enabledLabels := make([]string, 0, len(finding.enabled))
	related := []analysis.RelatedInformation{{
		Pos: finding.disabled.pos, End: finding.disabled.end,
		Message: "capability is false under " + finding.disabled.source.label,
	}}
	for _, enabled := range finding.enabled {
		position := pass.Fset.Position(enabled.pos)
		enabledLabels = append(enabledLabels, "["+enabled.source.label+"] at "+filepath.Base(position.Filename)+":"+strconv.Itoa(position.Line))
		related = append(related, analysis.RelatedInformation{
			Pos: enabled.pos, End: enabled.end,
			Message: "capability is true under " + enabled.source.label,
		})
	}
	var gatedFunctions, leaves []string
	seenFunctions, seenLeaves := make(map[string]bool), make(map[string]bool)
	for _, gate := range finding.gates {
		if !seenFunctions[gate.function.name] {
			seenFunctions[gate.function.name] = true
			gatedFunctions = append(gatedFunctions, gate.function.name)
			related = append(related, analysis.RelatedInformation{Pos: gate.position, Message: "constant-folded capability guard in " + gate.function.name})
		}
		for _, leaf := range gate.leaves {
			if !seenLeaves[leaf] {
				seenLeaves[leaf] = true
				leaves = append(leaves, leaf)
			}
		}
	}
	slices.Sort(gatedFunctions)
	slices.Sort(leaves)
	public := strings.Join(finding.public, ", ")
	if public == "" {
		public = "<no exported caller found>"
	}
	priority := "Architecture capability gap"
	evidence := "no nearby enabled same-dtype vector evidence"
	if len(finding.relatedCapabilities) > 0 || len(finding.relatedVectors) > 0 {
		priority = "HIGH-PRIORITY architecture capability gap"
		var pieces []string
		if len(finding.relatedCapabilities) > 0 {
			pieces = append(pieces, "enabled same-dtype capabilities "+strings.Join(finding.relatedCapabilities, ", "))
		}
		if len(finding.relatedVectors) > 0 {
			pieces = append(pieces, "local vector leaves "+strings.Join(finding.relatedVectors, ", "))
		}
		evidence = strings.Join(pieces, "; ")
	}
	pass.Report(analysis.Diagnostic{
		Pos: finding.disabled.pos, End: finding.disabled.end,
		Message: fmt.Sprintf("%s: %s is false under [%s] at %s:%d but true under %s; constant folding disables optimized branches in %s (leaves %s), reachable from exported operations %s. Nearby evidence: %s. Implement and separately gate the missing leaf, then validate every reachable route for alignment, tails, aliases, approximation/special values, and initialization before same-binary promotion (advisory, no automatic fix)",
			priority,
			finding.disabled.name,
			finding.disabled.source.label,
			filepath.Base(disabledPosition.Filename), disabledPosition.Line,
			strings.Join(enabledLabels, ", "),
			strings.Join(gatedFunctions, ", "),
			strings.Join(leaves, ", "),
			public,
			evidence,
		),
		Related: related,
	})
}

func ps6078ValidatedComments(group *ast.CommentGroup) bool {
	if group == nil {
		return false
	}
	for _, comment := range group.List {
		if strings.Contains(strings.ToLower(comment.Text), "perfscan:architecture-capability-validated") {
			return true
		}
	}
	return false
}

func ps6078ValidatedFunction(function *ast.FuncDecl) bool {
	return function.Doc != nil && ps6078ValidatedComments(function.Doc)
}
