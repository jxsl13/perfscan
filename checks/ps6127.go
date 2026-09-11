package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6127 implements owner issue #982. Configuration supplies the repository's
// reviewed recursive metadata policy; source still has to prove the mismatch.
var PS6127 = register(&lint.Check{
	ID: "PS6127", Category: "verify", Slug: "root-only-ignore-conflicts-with-recursive-metadata-policy",
	Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"recursiveMetadataIgnoreContracts"},
	Doc: lint.Documentation{
		Title: "a root-anchored metadata ignore conflicts with an any-depth policy",
		Text: `Incremental test selectors can unintentionally treat a reviewed metadata directory differently at the repository root and below packages. PS6127 reports only when an explicit project contract identifies an any-depth, non-embedded metadata directory and typed source proves both halves of the mismatch: an ignore builder joins configured entries to the repository root, and its matcher accepts only equality with that stored absolute entry or descendants of it.

The contract is an attestation, not source evidence. It must name the exact builder and matcher, their declared root/ignore fields, and affirm both recursive intent and non-embedded metadata-only contents. The analyzer resolves declared fields and standard-library calls through go/types; identifier spelling, comments, file extensions, and a directory name alone never establish safety. Component-based or recursive-regex matchers, intentional root-only contracts, ambiguous contracts, missing source bodies, and partial shapes remain silent.

There is no automatic fix and no measured speedup claim. Expanding an ignore can suppress required CI for embedded files, native assets, generated code, or unknown contents. Review the complete directory policy, add nested positive and code/asset negative tests, retain full-rerun precedence, and measure the actual selection fan-out before changing production configuration.`,
		Before: `ignored := filepath.Join(root, ".metadata")
if path == ignored || strings.HasPrefix(path, ignored+string(filepath.Separator)) {
	return true // nested pkg/.metadata is not matched
}`,
		After: `// Only after a repository-owned non-embedded metadata review:
for _, part := range strings.Split(filepath.ToSlash(path), "/") {
	if part == ".metadata" { return true }
}`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6127", Doc: "root-only metadata ignores conflicting with configured recursive intent", Run: runPS6127},
})

func runPS6127(pass *analysis.Pass) (any, error) {
	return runPS6127WithContracts(pass, config.Current().RecursiveMetadataIgnoreContracts)
}

func runPS6127WithContracts(pass *analysis.Pass, contracts []config.RecursiveMetadataIgnoreContract) (any, error) {
	counts := make(map[string]int)
	for _, contract := range contracts {
		if contract.Valid() {
			counts[contract.Matcher]++
		}
	}
	functions := make(map[string]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
				functions[ps6090FunctionID(object)] = function
			}
		}
	}
	for _, contract := range contracts {
		if !contract.Valid() || counts[contract.Matcher] != 1 {
			continue
		}
		builder, matcher := functions[contract.Builder], functions[contract.Matcher]
		if builder == nil || matcher == nil {
			continue
		}
		rootField, ignoreField, ok := ps6127RootBuilder(pass, builder, contract)
		if !ok || !ps6127RootMatcher(pass, functions, matcher, contract, rootField, ignoreField) || ps6127RecursiveDefault(pass, functions, builder, matcher, contract) != ps6127PolicyAbsent {
			continue
		}
		pass.Reportf(matcher.Name.Pos(), "%s: configured any-depth non-embedded metadata directory %q is filtered through a root-anchored ignore entry and its descendants only; nested contexts can bypass the policy and fan out CI—review complete contents and precedence, then use an explicit component/recursive matcher with nested metadata and embedded-code/asset controls (PS6127 advisory, no automatic fix or measured speedup claim)", contract.Name, contract.DirectoryName)
	}
	return nil, nil
}

type ps6127PolicyState uint8

const (
	ps6127PolicyAbsent ps6127PolicyState = iota
	ps6127PolicyCovered
	ps6127PolicyUnknown
)

func ps6127RecursiveDefault(pass *analysis.Pass, functions map[string]*ast.FuncDecl, builder, matcher *ast.FuncDecl, contract config.RecursiveMetadataIgnoreContract) ps6127PolicyState {
	// Keep every returned slot: a provider can return both root path strings
	// and regex strings matching the witness. Only the actual constructor
	// argument selects which slot matters, never map iteration/return order.
	providers := make(map[string][]int)
	unknownProviders := make(map[string][]int)
	for id, function := range functions {
		assigned := make(map[types.Object]bool)
		unknown := make(map[types.Object]bool)
		pointers := make(map[types.Object]types.Object)
		mutatingClosures := make(map[types.Object]bool)
		resultObjects := make(map[types.Object]int)
		resultIndex := 0
		if function.Type.Results != nil {
			for _, field := range function.Type.Results.List {
				for _, name := range field.Names {
					resultObjects[pass.TypesInfo.ObjectOf(name)] = resultIndex
					resultIndex++
				}
			}
		}
		valid := true
		for _, statement := range function.Body.List {
			switch value := statement.(type) {
			case *ast.AssignStmt:
				if len(value.Lhs) == 1 && len(value.Rhs) == 1 {
					lhs := ps6114ExprObject(pass, value.Lhs[0])
					rhs := ps6114ExprObject(pass, value.Rhs[0])
					if unary, ok := ps2110Unparen(value.Rhs[0]).(*ast.UnaryExpr); ok && unary.Op == token.AND {
						pointers[lhs] = ps6114ExprObject(pass, unary.X)
					} else {
						delete(pointers, lhs)
					}
					if star, ok := ps2110Unparen(value.Lhs[0]).(*ast.StarExpr); ok && assigned[pointers[ps6114ExprObject(pass, star.X)]] {
						valid = false
					}
					if literal, ok := ps2110Unparen(value.Rhs[0]).(*ast.FuncLit); ok {
						mutatingClosures[lhs] = ps6127WritesTracked(pass, literal.Body, assigned)
					} else {
						mutatingClosures[lhs] = mutatingClosures[rhs]
					}
				}
				for index, lhs := range value.Lhs {
					rhs := ast.Expr(nil)
					if index < len(value.Rhs) {
						rhs = value.Rhs[index]
					}
					assigned[ps6114ExprObject(pass, lhs)] = rhs != nil && ps6127ContainsRecursivePattern(pass, rhs, contract.DirectoryName)
					unknown[ps6114ExprObject(pass, lhs)] = rhs != nil && ps6127UnknownPolicyExpr(pass, rhs)
				}
			case *ast.ReturnStmt:
				if !valid {
					continue
				}
				if len(value.Results) == 0 {
					for object, index := range resultObjects {
						if assigned[object] {
							providers[id] = append(providers[id], index)
						}
						if unknown[object] && ps6127StringSliceResult(pass, function, index) {
							unknownProviders[id] = append(unknownProviders[id], index)
						}
					}
				} else {
					for index, result := range value.Results {
						if assigned[ps6114ExprObject(pass, result)] && ps6127StringSliceResult(pass, function, index) {
							providers[id] = append(providers[id], index)
						}
						if unknown[ps6114ExprObject(pass, result)] && ps6127StringSliceResult(pass, function, index) {
							unknownProviders[id] = append(unknownProviders[id], index)
						}
					}
				}
			default:
				if deferStatement, ok := statement.(*ast.DeferStmt); ok && mutatingClosures[ps6114ExprObject(pass, deferStatement.Call.Fun)] {
					valid = false
				}
				if ps6127WritesTracked(pass, statement, assigned) {
					valid = false
				}
			}
		}
	}
	// A recursive default is relevant only when it is carried through a call
	// chain into the configured builder, and that builder compiles the same
	// parameter into a regexp field consumed by the configured matcher.
	regexParameter, regexField := ps6127CompiledRegexParameter(pass, builder, matcher)
	if regexParameter < 0 {
		return ps6127PolicyAbsent
	}
	for _, function := range functions {
		values := make(map[types.Object]bool)
		unknownValues := make(map[types.Object]bool)
		built := make(map[types.Object]bool)
		unknownBuilt := make(map[types.Object]bool)
		constructed := make(map[types.Object]bool)
		found := false
		for _, statement := range function.Body.List {
			ps6127DefaultStatementEffects(pass, functions, statement, regexField, built, unknownBuilt, constructed)
			switch value := statement.(type) {
			case *ast.AssignStmt:
				var dependency types.Object
				for _, rhs := range value.Rhs {
					ast.Inspect(rhs, func(node ast.Node) bool {
						if identifier, ok := node.(*ast.Ident); ok {
							if object := pass.TypesInfo.ObjectOf(identifier); constructed[object] {
								dependency = object
							}
						}
						return true
					})
				}
				if dependency != nil {
					// An unsupported assignment can return/retain an alias to an
					// existing config. Never erase an exposure's Unknown on the LHS
					// or leave another potentially aliased return value as Absent.
					for object := range constructed {
						built[object], unknownBuilt[object] = false, true
					}
				}
				for _, lhs := range value.Lhs {
					object := ps6114ExprObject(pass, lhs)
					values[object] = false
					unknownValues[object] = false
					built[object] = false
					unknownBuilt[object] = false
					delete(constructed, object)
					if _, identifier := ps2110Unparen(lhs).(*ast.Ident); identifier && object != nil && dependency != nil && types.Identical(object.Type(), dependency.Type()) {
						constructed[object], unknownBuilt[object] = true, true
					}
					if selector := ps6127Selector(lhs); selector != nil {
						base := ps6114ExprObject(pass, selector.X)
						if constructed[base] {
							built[base] = false
							unknownBuilt[base] = true
							if len(value.Lhs) == 1 && len(value.Rhs) == 1 && ps6127AnyFieldObject(pass, lhs) == regexField && ps6114ExprObject(pass, value.Rhs[0]) == types.Universe.Lookup("nil") {
								unknownBuilt[base] = false
							}
						}
					}
				}
				if len(value.Rhs) == 1 {
					if call, ok := ps2110Unparen(value.Rhs[0]).(*ast.CallExpr); ok {
						for _, p := range providers[ps6127CallID(pass, call)] {
							if p < len(value.Lhs) {
								values[ps6114ExprObject(pass, value.Lhs[p])] = true
							}
						}
						for _, p := range unknownProviders[ps6127CallID(pass, call)] {
							if p < len(value.Lhs) {
								unknownValues[ps6114ExprObject(pass, value.Lhs[p])] = true
							}
						}
						if ps6127CallID(pass, call) == contract.Builder && regexParameter < len(call.Args) && len(value.Lhs) > 0 {
							argument := ps6114ExprObject(pass, call.Args[regexParameter])
							result := ps6114ExprObject(pass, value.Lhs[0])
							built[result] = values[argument]
							unknownBuilt[result] = unknownValues[argument] || dependency != nil
							constructed[result] = true
						}
					}
				}
			case *ast.ReturnStmt:
				for _, result := range value.Results {
					if unknownBuilt[ps6114ExprObject(pass, result)] {
						return ps6127PolicyUnknown
					}
					if built[ps6114ExprObject(pass, result)] {
						found = true
					}
				}
			default:
				if ps6127WritesTracked(pass, statement, values) {
					for object := range values {
						values[object] = false
					}
				}
				ast.Inspect(statement, func(node ast.Node) bool {
					assignment, ok := node.(*ast.AssignStmt)
					if !ok {
						return true
					}
					for _, lhs := range assignment.Lhs {
						if selector := ps6127Selector(lhs); selector != nil {
							base := ps6114ExprObject(pass, selector.X)
							if constructed[base] {
								built[base], unknownBuilt[base] = false, true
							}
						}
					}
					return true
				})
			}
		}
		if found {
			return ps6127PolicyCovered
		}
	}
	if ps6127UnknownDefaultConnection(pass, functions, unknownProviders, contract.Builder, regexParameter) {
		return ps6127PolicyUnknown
	}
	return ps6127PolicyAbsent
}

func ps6127UnknownDefaultConnection(pass *analysis.Pass, functions map[string]*ast.FuncDecl, providers map[string][]int, builderID string, argumentIndex int) bool {
	for _, function := range functions {
		unknown := make(map[types.Object]bool)
		hasUnknownProvider := false
		for _, statement := range function.Body.List {
			assignment, ok := statement.(*ast.AssignStmt)
			if !ok || len(assignment.Rhs) != 1 {
				continue
			}
			call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			if !ok {
				continue
			}
			if slots := providers[ps6127CallID(pass, call)]; len(slots) > 0 {
				for _, index := range slots {
					if index < len(assignment.Lhs) {
						unknown[ps6114ExprObject(pass, assignment.Lhs[index])] = true
						hasUnknownProvider = true
					}
				}
				continue
			}
			if ps6127CallID(pass, call) == builderID && argumentIndex < len(call.Args) && (unknown[ps6114ExprObject(pass, call.Args[argumentIndex])] || hasUnknownProvider) {
				return true
			}
		}
	}
	return false
}

func ps6127StringSliceResult(pass *analysis.Pass, function *ast.FuncDecl, index int) bool {
	signature, _ := pass.TypesInfo.Defs[function.Name].Type().(*types.Signature)
	if signature == nil || index >= signature.Results().Len() {
		return false
	}
	slice, ok := types.Unalias(signature.Results().At(index).Type()).Underlying().(*types.Slice)
	return ok && types.Identical(slice.Elem(), types.Typ[types.String])
}

func ps6127UnknownPolicyExpr(pass *analysis.Pass, expression ast.Expr) bool {
	unknown := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if ps6127CallID(pass, call) == "strings.ReplaceAll" && len(call.Args) == 3 {
				known := true
				for _, argument := range call.Args {
					value := pass.TypesInfo.Types[argument].Value
					known = known && value != nil && value.Kind() == constant.String
				}
				if known {
					return false
				}
			}
			unknown = true
			return false
		}
		return !unknown
	})
	return unknown
}

func ps6127WritesTracked(pass *analysis.Pass, node ast.Node, tracked map[types.Object]bool) bool {
	writes := false
	pointers := make(map[types.Object]types.Object)
	ast.Inspect(node, func(n ast.Node) bool {
		assignment, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 {
			if unary, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.UnaryExpr); ok && unary.Op == token.AND {
				pointers[ps6114ExprObject(pass, assignment.Lhs[0])] = ps6114ExprObject(pass, unary.X)
			}
		}
		for _, lhs := range assignment.Lhs {
			if tracked[ps6114ExprObject(pass, lhs)] {
				writes = true
			}
			if star, ok := ps2110Unparen(lhs).(*ast.StarExpr); ok && tracked[pointers[ps6114ExprObject(pass, star.X)]] {
				writes = true
			}
		}
		return !writes
	})
	return writes
}

func ps6127ContainsRecursivePattern(pass *analysis.Pass, expression ast.Expr, directory string) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if _, ok := node.(*ast.CallExpr); ok {
			return false
		}
		expr, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		value := pass.TypesInfo.Types[expr].Value
		if value == nil || value.Kind() != constant.String {
			return true
		}
		pattern := constant.StringVal(value)
		// This proves only the configured concrete nested counterexample. It is
		// not a universal regular-language claim: unsupported/nonconstant policy
		// remains unknown and suppresses neither side of the proof.
		if matched, err := regexp.MatchString(pattern, "pkg/"+directory+"/entry"); err == nil && matched {
			found = true
		}
		return !found
	})
	return found
}

func ps6127CompiledRegexParameter(pass *analysis.Pass, builder, matcher *ast.FuncDecl) (int, *types.Var) {
	params := make(map[types.Object]int)
	position := 0
	for _, field := range builder.Type.Params.List {
		for _, name := range field.Names {
			params[pass.TypesInfo.ObjectOf(name)] = position
			position++
		}
	}
	consumed := make(map[*types.Var]bool)
	var receiver, input types.Object
	if matcher.Recv != nil && len(matcher.Recv.List) == 1 && len(matcher.Recv.List[0].Names) == 1 && matcher.Type.Params != nil && len(matcher.Type.Params.List) > 0 && len(matcher.Type.Params.List[0].Names) == 1 {
		receiver = pass.TypesInfo.ObjectOf(matcher.Recv.List[0].Names[0])
		input = pass.TypesInfo.ObjectOf(matcher.Type.Params.List[0].Names[0])
	}
	ast.Inspect(matcher.Body, func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok {
			return true
		}
		field := ps6127AnyFieldObject(pass, loop.X)
		selector := ps6127Selector(loop.X)
		if field == nil || selector == nil || ps6114ExprObject(pass, selector.X) != receiver {
			return true
		}
		item := ps6114ExprObject(pass, loop.Value)
		stable := true
		for _, statement := range loop.Body.List {
			if _, ok := statement.(*ast.BranchStmt); ok {
				stable = false
				break
			}
			if assignment, ok := statement.(*ast.AssignStmt); ok {
				for _, lhs := range assignment.Lhs {
					if ps6114ExprObject(pass, lhs) == item {
						stable = false
					}
				}
			}
			branch, ok := statement.(*ast.IfStmt)
			if !ok || !stable || !ps6127ReturnsTrue(pass, branch.Body.List) {
				continue
			}
			call, ok := ps2110Unparen(branch.Cond).(*ast.CallExpr)
			if ok && len(call.Args) == 1 && ps6114ExprObject(pass, call.Args[0]) == input && ps6127Method(pass, call, "regexp", "Regexp", "MatchString") && ps6114ExprObject(pass, call.Fun.(*ast.SelectorExpr).X) == item {
				consumed[field] = true
			}
		}
		return false
	})
	found := -1
	foundPos := token.NoPos
	var foundField *types.Var
	ast.Inspect(builder.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || ps6127CallID(pass, call) != "regexp.Compile" || len(call.Args) != 1 {
			return true
		}
		parameter, ok := params[ps6114ExprObject(pass, call.Args[0])]
		if !ok {
			// Owner compilation is inside a literal ranging over the parameter.
			for _, parent := range params {
				_ = parent
			}
			return true
		}
		ast.Inspect(builder.Body, func(n ast.Node) bool {
			assignment, ok := n.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				return true
			}
			field := ps6127AnyFieldObject(pass, assignment.Lhs[0])
			if field != nil && consumed[field] && ps6127ContainsObject(pass, assignment.Rhs[0], ps6114ExprObject(pass, call.Args[0])) {
				found = parameter
				foundField = field
				foundPos = assignment.Pos()
			}
			return true
		})
		return true
	})
	// Handle the owner's local compile helper: identify its pats parameter and
	// dst field at the call site, then bind that argument back to builder params.
	if found < 0 {
		helpers := make(map[types.Object]*ast.FuncLit)
		ast.Inspect(builder.Body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				return true
			}
			literal, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.FuncLit)
			if ok && ps6127RegexCompiler(pass, literal) {
				helpers[ps6114ExprObject(pass, assignment.Lhs[0])] = literal
			}
			return true
		})
		ast.Inspect(builder.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			if helpers[ps6114ExprObject(pass, call.Fun)] == nil {
				return true
			}
			parameter, ok := params[ps6114ExprObject(pass, call.Args[0])]
			unary, ok2 := ps2110Unparen(call.Args[1]).(*ast.UnaryExpr)
			if ok && ok2 && unary.Op == token.AND && consumed[ps6127AnyFieldObject(pass, unary.X)] {
				found = parameter
				foundPos = call.Pos()
				foundField = ps6127AnyFieldObject(pass, unary.X)
			}
			return true
		})
	}
	if found >= 0 {
		ast.Inspect(builder.Body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || assignment.Pos() <= foundPos {
				return true
			}
			for _, lhs := range assignment.Lhs {
				if ps6127AnyFieldObject(pass, lhs) == foundField {
					found = -1
				}
			}
			return found >= 0
		})
	}
	return found, foundField
}

func ps6127RegexCompiler(pass *analysis.Pass, literal *ast.FuncLit) bool {
	if literal.Type.Params == nil || len(literal.Type.Params.List) < 2 || len(literal.Type.Params.List[0].Names) != 1 || len(literal.Type.Params.List[1].Names) != 1 {
		return false
	}
	pats := pass.TypesInfo.ObjectOf(literal.Type.Params.List[0].Names[0])
	dst := pass.TypesInfo.ObjectOf(literal.Type.Params.List[1].Names[0])
	compiled := make(map[types.Object]bool)
	validRange := false
	appendPos := token.NoPos
	flow := ps6122NewFlow(pass, literal.Body)
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok || ps6114ExprObject(pass, loop.X) != pats {
			return true
		}
		item := ps6114ExprObject(pass, loop.Value)
		ast.Inspect(loop.Body, func(n ast.Node) bool {
			assignment, ok := n.(*ast.AssignStmt)
			if !ok || len(assignment.Rhs) != 1 {
				return true
			}
			for _, lhs := range assignment.Lhs {
				delete(compiled, ps6114ExprObject(pass, lhs))
			}
			call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			if ok && ps6127CallID(pass, call) == "regexp.Compile" && len(call.Args) == 1 && ps6114ExprObject(pass, call.Args[0]) == item && len(assignment.Lhs) > 0 {
				compiled[ps6114ExprObject(pass, assignment.Lhs[0])] = true
			}
			if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				return true
			}
			star, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.StarExpr)
			appendCall, ok2 := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
			id, ok3 := func() (*ast.Ident, bool) {
				if !ok2 {
					return nil, false
				}
				x, yes := appendCall.Fun.(*ast.Ident)
				return x, yes
			}()
			if ok && ps6114ExprObject(pass, star.X) == dst && ok2 && ok3 && id.Name == "append" && pass.TypesInfo.Uses[id] == types.Universe.Lookup("append") && len(appendCall.Args) == 2 && compiled[ps6114ExprObject(pass, appendCall.Args[1])] && ps6122CanEnter(pass, literal.Body, flow.parents, assignment.Pos()) {
				validRange = true
				appendPos = assignment.Pos()
			}
			return true
		})
		return false
	})
	if validRange {
		aliases := map[types.Object]bool{dst: true}
		ast.Inspect(literal.Body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || assignment.Pos() <= appendPos {
				return true
			}
			if len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 {
				lhs, rhs := ps6114ExprObject(pass, assignment.Lhs[0]), ps6114ExprObject(pass, assignment.Rhs[0])
				aliases[lhs] = aliases[rhs]
			}
			for _, lhs := range assignment.Lhs {
				if star, ok := ps2110Unparen(lhs).(*ast.StarExpr); ok && aliases[ps6114ExprObject(pass, star.X)] {
					validRange = false
				}
			}
			return validRange
		})
	}
	return validRange
}

func ps6127ContainsObject(pass *analysis.Pass, expression ast.Expr, want types.Object) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		expr, ok := node.(ast.Expr)
		if ok && ps6114ExprObject(pass, expr) == want {
			found = true
		}
		return !found
	})
	return found
}

func ps6127RootBuilder(pass *analysis.Pass, function *ast.FuncDecl, contract config.RecursiveMetadataIgnoreContract) (*types.Var, *types.Var, bool) {
	state := &ps6127BuilderState{pass: pass, contract: contract, inputs: make(map[types.Object]bool), roots: make(map[types.Object]bool), entries: make(map[types.Object]int), instances: make(map[types.Object]int), stored: []bool{false}, rooted: []bool{false}, closures: make(map[types.Object]*ast.FuncLit), bound: make(map[types.Object]int), pointers: make(map[types.Object]int)}
	if function.Type.Params == nil || len(function.Type.Params.List) < 2 {
		return nil, nil, false
	}
	for _, name := range function.Type.Params.List[0].Names {
		state.roots[pass.TypesInfo.ObjectOf(name)] = true
	}
	for _, field := range function.Type.Params.List[1:] {
		for _, name := range field.Names {
			state.inputs[pass.TypesInfo.ObjectOf(name)] = true
		}
	}
	state.walk(function.Body.List)
	ok := !state.uncertain && state.returned != 0 && state.stored[state.returned] && state.rooted[state.returned] && state.ignoreField != nil && state.rootField != nil && state.rootField.Parent() == state.ignoreField.Parent()
	return state.rootField, state.ignoreField, ok
}

type ps6127BuilderState struct {
	pass                   *analysis.Pass
	contract               config.RecursiveMetadataIgnoreContract
	inputs                 map[types.Object]bool
	roots                  map[types.Object]bool
	entries                map[types.Object]int // 1 input/directory entry; 2 same entry rooted by Join.
	instances              map[types.Object]int
	stored                 []bool // Dense allocation identities; index zero is unknown.
	rooted                 []bool
	closures               map[types.Object]*ast.FuncLit
	bound                  map[types.Object]int
	pointers               map[types.Object]int
	next, returned         int
	callDepth              int
	uncertain              bool
	rootField, ignoreField *types.Var
}

func (state *ps6127BuilderState) walk(statements []ast.Stmt) bool {
	for _, statement := range statements {
		switch value := statement.(type) {
		case *ast.AssignStmt:
			state.assign(value)
		case *ast.RangeStmt:
			state.invalidateCall(value.X)
			if state.inputs[ps6114ExprObject(state.pass, value.X)] {
				state.entries[ps6114ExprObject(state.pass, value.Value)] = 1
				state.walk(value.Body.List)
			} else if ps6122RangeTrips(state.pass, value) > 0 {
				state.walk(value.Body.List)
			}
		case *ast.IfStmt:
			if value.Init != nil {
				state.walk([]ast.Stmt{value.Init})
			}
			state.invalidateCall(value.Cond)
			condition, known := state.condition(value.Cond)
			if !known {
				thenState, elseState := state.clone(), state.clone()
				thenDone := thenState.walk(value.Body.List)
				// Allocate distinct identities in mutually exclusive arms too.
				elseState.next = thenState.next
				elseState.stored = append(elseState.stored, make([]bool, len(thenState.stored)-len(elseState.stored))...)
				elseState.rooted = append(elseState.rooted, make([]bool, len(thenState.rooted)-len(elseState.rooted))...)
				elseDone := false
				if value.Else != nil {
					elseDone = elseState.walkElse(value.Else)
				}
				state.join(thenState, elseState, thenDone, elseDone)
				if thenDone && elseDone {
					return true
				}
			} else if condition {
				if state.walk(value.Body.List) {
					return true
				}
			} else if value.Else != nil && state.walkElse(value.Else) {
				return true
			}
		case *ast.ForStmt:
			if value.Init != nil {
				state.walk([]ast.Stmt{value.Init})
			}
			if ps6127LoopRuns(state.pass, value) {
				state.walk(value.Body.List)
				if value.Post != nil {
					state.walk([]ast.Stmt{value.Post})
				}
			}
		case *ast.BlockStmt:
			if state.walk(value.List) {
				return true
			}
		case *ast.ReturnStmt:
			for _, result := range value.Results {
				state.invalidateCall(result)
			}
			if state.callDepth != 0 {
				return true
			}
			if len(value.Results) >= 1 {
				state.returned = state.instances[ps6114ExprObject(state.pass, value.Results[0])]
				if state.returned == 0 && ps6114ExprObject(state.pass, value.Results[0]) != types.Universe.Lookup("nil") {
					state.uncertain = true
				}
			} else {
				state.uncertain = true
			}
			return true
		case *ast.ExprStmt:
			if call, ok := ps2110Unparen(value.X).(*ast.CallExpr); ok {
				if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
					state.walkClosure(literal)
				} else if literal := state.closures[ps6114ExprObject(state.pass, call.Fun)]; literal != nil {
					state.walkClosure(literal)
				} else if instance := state.bound[ps6114ExprObject(state.pass, call.Fun)]; instance != 0 {
					state.stored[instance], state.rooted[instance] = false, false
				}
			}
			state.invalidateCall(value.X)
		case *ast.DeferStmt:
			state.invalidateCall(value.Call)
			state.invalidateClosure(value.Call)
		case *ast.GoStmt:
			state.invalidateClosure(value.Call)
		case *ast.SwitchStmt:
			if value.Init != nil {
				state.walk([]ast.Stmt{value.Init})
			}
			if value.Tag != nil && state.pass.TypesInfo.Types[value.Tag].Value == nil {
				state.uncertain = true
				break
			}
			for _, raw := range value.Body.List {
				for _, expression := range raw.(*ast.CaseClause).List {
					if state.pass.TypesInfo.Types[expression].Value == nil {
						state.uncertain = true
					}
				}
			}
			if state.uncertain {
				break
			}
			selected := false
			for _, raw := range value.Body.List {
				clause := raw.(*ast.CaseClause)
				if len(clause.List) != 0 && ps6127CaseMayRun(state.pass, value.Tag, clause.List) {
					if state.walk(clause.Body) {
						return true
					}
					selected = true
					break
				}
			}
			if !selected {
				for _, raw := range value.Body.List {
					clause := raw.(*ast.CaseClause)
					if len(clause.List) == 0 {
						if state.walk(clause.Body) {
							return true
						}
						break
					}
				}
			}
		case *ast.SelectStmt:
			state.uncertain = true
		case *ast.TypeSwitchStmt:
			state.uncertain = true
		case *ast.LabeledStmt:
			state.walk([]ast.Stmt{value.Stmt})
		case *ast.DeclStmt:
			declaration, ok := value.Decl.(*ast.GenDecl)
			if ok {
				for _, spec := range declaration.Specs {
					values, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range values.Names {
						if i < len(values.Values) {
							state.assign(&ast.AssignStmt{Lhs: []ast.Expr{name}, Rhs: []ast.Expr{values.Values[i]}, Tok: token.DEFINE})
						}
					}
				}
			}
		case *ast.BranchStmt:
			if value.Tok == token.GOTO {
				state.returned = 0
				return true
			}
		}
	}
	return false
}

func (state *ps6127BuilderState) clone() *ps6127BuilderState {
	result := *state
	result.inputs = maps.Clone(state.inputs)
	result.roots = maps.Clone(state.roots)
	result.entries = maps.Clone(state.entries)
	result.instances = maps.Clone(state.instances)
	result.stored = slices.Clone(state.stored)
	result.rooted = slices.Clone(state.rooted)
	result.closures = maps.Clone(state.closures)
	result.bound = maps.Clone(state.bound)
	result.pointers = maps.Clone(state.pointers)
	return &result
}

func (state *ps6127BuilderState) walkClosure(literal *ast.FuncLit) {
	captures := false
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			object := state.pass.TypesInfo.ObjectOf(identifier)
			captures = captures || state.instances[object] != 0 || state.pointers[object] != 0 || state.bound[object] != 0 || state.closures[object] != nil || state.roots[object] || state.inputs[object] || state.entries[object] != 0
		}
		return !captures
	})
	if !captures {
		// Formal-parameter effects are handled at the actual arguments by
		// invalidateCall. A non-capturing helper's local completion cannot
		// discard or restore facts about an unrelated caller-owned instance.
		return
	}
	if state.callDepth >= 8 {
		state.uncertain = true
		return
	}
	returned := state.returned
	state.callDepth++
	state.walk(literal.Body.List)
	state.callDepth--
	state.returned = returned
}

func (state *ps6127BuilderState) join(left, right *ps6127BuilderState, leftDone, rightDone bool) {
	// A nil/error return cannot produce the configured instance. A returned
	// instance or ambiguous location needs a richer disjunction; abstain rather
	// than letting one arm's locations explain the other arm's field writes.
	state.uncertain = left.uncertain || right.uncertain
	if state.callDepth != 0 && leftDone != rightDone {
		// One arm returns to the caller while the other continues the closure.
		// A single joined continuation cannot represent both completion points.
		state.uncertain = true
	}
	if state.callDepth == 0 {
		state.uncertain = state.uncertain || (leftDone && left.returned != 0) || (rightDone && right.returned != 0)
	}
	if state.callDepth == 0 && leftDone && left.returned == 0 && !left.uncertain {
		*state = *right
		return
	}
	if state.callDepth == 0 && rightDone && right.returned == 0 && !right.uncertain {
		*state = *left
		return
	}
	state.uncertain = state.uncertain || !ps6127SameFacts(left.instances, right.instances) ||
		!ps6127SameFacts(left.pointers, right.pointers) || !ps6127SameFacts(left.bound, right.bound) ||
		!ps6127SameFacts(left.closures, right.closures) || left.rootField != right.rootField || left.ignoreField != right.ignoreField
	state.inputs = ps6127CommonFacts(left.inputs, right.inputs)
	state.roots = ps6127CommonFacts(left.roots, right.roots)
	state.entries = ps6127CommonFacts(left.entries, right.entries)
	state.instances = ps6127CommonFacts(left.instances, right.instances)
	state.pointers = ps6127CommonFacts(left.pointers, right.pointers)
	state.bound = ps6127CommonFacts(left.bound, right.bound)
	state.closures = ps6127CommonFacts(left.closures, right.closures)
	state.rootField, state.ignoreField = left.rootField, left.ignoreField
	state.next = right.next
	state.stored, state.rooted = make([]bool, state.next+1), make([]bool, state.next+1)
	for instance := range left.stored {
		state.stored[instance] = left.stored[instance] && right.stored[instance]
		state.rooted[instance] = left.rooted[instance] && right.rooted[instance]
	}
}

func ps6127CommonFacts[V comparable](left, right map[types.Object]V) map[types.Object]V {
	result := make(map[types.Object]V)
	for object, value := range left {
		if other, ok := right[object]; ok && value == other {
			result[object] = value
		}
	}
	return result
}

func ps6127SameFacts[V comparable](left, right map[types.Object]V) bool {
	// These maps use the zero value for unknown/no binding. An explicit zero
	// and an absent key are the same abstract location, not an alias conflict.
	for object, value := range left {
		if right[object] != value {
			return false
		}
	}
	for object, value := range right {
		if left[object] != value {
			return false
		}
	}
	return true
}

func (state *ps6127BuilderState) condition(expression ast.Expr) (bool, bool) {
	if truth, known := ps6122Bool(state.pass, expression); known {
		return truth, true
	}
	// The witness is the configured single relative directory component, not
	// every arbitrary caller-supplied ignore string. Its IsAbs branch is known.
	if unary, ok := ps2110Unparen(expression).(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		if call, ok := ps2110Unparen(unary.X).(*ast.CallExpr); ok && ps6127CallID(state.pass, call) == "path/filepath.IsAbs" && len(call.Args) == 1 && state.entryExpr(call.Args[0]) == 1 {
			return true, true
		}
	}
	return false, false
}

func (state *ps6127BuilderState) walkElse(statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.BlockStmt:
		return state.walk(value.List)
	case *ast.IfStmt:
		return state.walk([]ast.Stmt{value})
	}
	return false
}

func ps6127LoopRuns(pass *analysis.Pass, loop *ast.ForStmt) bool {
	if loop.Cond == nil {
		return true
	}
	truth, known := ps6122Bool(pass, loop.Cond)
	return !known || truth
}

func ps6127CaseMayRun(pass *analysis.Pass, tag ast.Expr, expressions []ast.Expr) bool {
	if tag == nil {
		for _, expression := range expressions {
			if truth, known := ps6122Bool(pass, expression); !known || truth {
				return true
			}
		}
		return false
	}
	tagValue := pass.TypesInfo.Types[tag].Value
	if tagValue == nil {
		return true
	}
	for _, expression := range expressions {
		value := pass.TypesInfo.Types[expression].Value
		if value == nil || constant.Compare(tagValue, token.EQL, value) {
			return true
		}
	}
	return false
}

func (state *ps6127BuilderState) assign(assignment *ast.AssignStmt) {
	if len(assignment.Lhs) == 1 {
		if _, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.StarExpr); ok {
			state.clear(assignment.Lhs[0])
			return
		}
	}
	if len(assignment.Lhs) == 2 && len(assignment.Rhs) == 1 {
		if call, ok := assignment.Rhs[0].(*ast.CallExpr); ok && ps6127CallID(state.pass, call) == "path/filepath.Abs" && len(call.Args) == 1 && state.rootExpr(call.Args[0]) {
			state.roots[ps6114ExprObject(state.pass, assignment.Lhs[0])] = true
			return
		}
	}
	if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		// Parallel assignment observes all RHS values first, but every target gets
		// a new version. Unknown tuple-producing calls are effects as well.
		for _, lhs := range assignment.Lhs {
			state.clear(lhs)
		}
		for _, rhs := range assignment.Rhs {
			state.invalidateCall(rhs)
		}
		return
	}
	lhs, rhs := assignment.Lhs[0], ps2110Unparen(assignment.Rhs[0])
	state.invalidateCall(rhs)
	if field := ps6127FieldObject(state.pass, lhs, state.contract.IgnoreField); field != nil {
		instance := state.instances[ps6114ExprObject(state.pass, ps6127Selector(lhs).X)]
		state.ignoreField = field
		if id, ok := rhs.(*ast.Ident); ok && id.Name == "nil" {
			state.stored[instance] = false
			return
		}
		call, ok := rhs.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			state.stored[instance] = false
			return
		}
		id, builtin := call.Fun.(*ast.Ident)
		state.stored[instance] = builtin && id.Name == "append" && state.pass.TypesInfo.Uses[id] == types.Universe.Lookup("append") && ps6127FieldObject(state.pass, call.Args[0], state.contract.IgnoreField) == field && state.entryExpr(call.Args[len(call.Args)-1]) == 2
		return
	}
	if field := ps6127FieldObject(state.pass, lhs, state.contract.RootField); field != nil {
		instance := state.instances[ps6114ExprObject(state.pass, ps6127Selector(lhs).X)]
		state.rootField = field
		state.rooted[instance] = state.rootExpr(rhs)
		return
	}
	object := ps6114ExprObject(state.pass, lhs)
	delete(state.closures, object)
	delete(state.bound, object)
	delete(state.pointers, object)
	if unary, ok := rhs.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		if selector := ps6127Selector(unary.X); selector != nil {
			state.pointers[object] = state.instances[ps6114ExprObject(state.pass, selector.X)]
		}
	}
	if source := ps6114ExprObject(state.pass, rhs); state.pointers[source] != 0 {
		state.pointers[object] = state.pointers[source]
	}
	if literal, ok := rhs.(*ast.FuncLit); ok {
		state.closures[object] = literal
	} else if source := ps6114ExprObject(state.pass, rhs); state.closures[source] != nil {
		state.closures[object] = state.closures[source]
	}
	if selector, ok := rhs.(*ast.SelectorExpr); ok {
		state.bound[object] = state.instances[ps6114ExprObject(state.pass, selector.X)]
	} else if source := ps6114ExprObject(state.pass, rhs); state.bound[source] != 0 {
		state.bound[object] = state.bound[source]
	}
	root, input, entry, instance := false, false, 0, 0
	if source := ps6114ExprObject(state.pass, rhs); source != nil {
		root, input, entry, instance = state.roots[source], state.inputs[source], state.entries[source], state.instances[source]
	}
	if call, ok := rhs.(*ast.CallExpr); ok {
		switch ps6127CallID(state.pass, call) {
		case "path/filepath.Clean":
			if len(call.Args) == 1 {
				entry = state.entryExpr(call.Args[0])
			}
		case "path/filepath.Join":
			if len(call.Args) >= 2 && state.rootExpr(call.Args[0]) && state.entryExpr(call.Args[1]) == 1 {
				entry = 2
			}
		}
	}
	delete(state.roots, object)
	delete(state.inputs, object)
	delete(state.entries, object)
	delete(state.instances, object)
	state.roots[object], state.inputs[object], state.entries[object], state.instances[object] = root, input, entry, instance
	if literal, ok := rhs.(*ast.UnaryExpr); ok && literal.Op == token.AND {
		if composite, ok := literal.X.(*ast.CompositeLit); ok {
			state.next++
			state.stored = append(state.stored, false)
			state.rooted = append(state.rooted, false)
			state.instances[object] = state.next
			for _, element := range composite.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				id, keyOK := pair.Key.(*ast.Ident)
				if ok && keyOK && id.Name == state.contract.RootField && state.rootExpr(pair.Value) {
					state.rootField, _ = state.pass.TypesInfo.ObjectOf(id).(*types.Var)
					state.rooted[state.next] = true
				}
			}
		}
	}
}

func (state *ps6127BuilderState) clear(expression ast.Expr) {
	expression = ps2110Unparen(expression)
	if star, ok := expression.(*ast.StarExpr); ok {
		if instance := state.pointers[ps6114ExprObject(state.pass, star.X)]; instance != 0 {
			state.stored[instance], state.rooted[instance] = false, false
			return
		}
		instance := state.instances[ps6114ExprObject(state.pass, star.X)]
		if instance != 0 {
			state.stored[instance], state.rooted[instance] = false, false
		}
		return
	}
	if field := ps6127FieldObject(state.pass, expression, state.contract.IgnoreField); field != nil {
		selector := ps6127Selector(expression)
		state.stored[state.instances[ps6114ExprObject(state.pass, selector.X)]] = false
		return
	}
	if field := ps6127FieldObject(state.pass, expression, state.contract.RootField); field != nil {
		selector := ps6127Selector(expression)
		state.rooted[state.instances[ps6114ExprObject(state.pass, selector.X)]] = false
		return
	}
	object := ps6114ExprObject(state.pass, expression)
	delete(state.roots, object)
	delete(state.entries, object)
	delete(state.instances, object)
	delete(state.inputs, object)
}

func (state *ps6127BuilderState) invalidateCall(expression ast.Expr) {
	ast.Inspect(expression, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if identifier, ok := call.Fun.(*ast.Ident); ok && state.pass.TypesInfo.Uses[identifier] == types.Universe.Lookup(identifier.Name) {
			return true
		}
		if literal := state.closures[ps6114ExprObject(state.pass, call.Fun)]; literal != nil {
			state.walkClosure(literal)
		}
		if instance := state.bound[ps6114ExprObject(state.pass, call.Fun)]; instance != 0 {
			state.stored[instance], state.rooted[instance] = false, false
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			state.invalidateArgument(selector.X)
		}
		if id := ps6127CallID(state.pass, call); strings.HasPrefix(id, "path/filepath.") || id == "len" || id == "append" {
			return true
		}
		for _, argument := range call.Args {
			state.invalidateArgument(argument)
		}
		return true
	})
}

func (state *ps6127BuilderState) invalidateArgument(argument ast.Expr) {
	expression := ps2110Unparen(argument)
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		expression = ps2110Unparen(unary.X)
	}
	if selector := ps6127Selector(expression); selector != nil {
		field := ps6127AnyFieldObject(state.pass, selector)
		if field != state.rootField && field != state.ignoreField {
			return
		}
		instance := state.instances[ps6114ExprObject(state.pass, selector.X)]
		if instance != 0 {
			state.stored[instance], state.rooted[instance] = false, false
		}
		return
	}
	instance := state.instances[ps6114ExprObject(state.pass, expression)]
	if instance != 0 {
		state.stored[instance], state.rooted[instance] = false, false
	}
}

func (state *ps6127BuilderState) invalidateClosure(call *ast.CallExpr) {
	literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit)
	if !ok {
		state.invalidateCall(call)
		return
	}
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		expr, ok := node.(ast.Expr)
		if ok {
			state.invalidateArgument(expr)
		}
		return true
	})
}

func (state *ps6127BuilderState) rootExpr(expression ast.Expr) bool {
	return state.roots[ps6114ExprObject(state.pass, expression)]
}

func (state *ps6127BuilderState) entryExpr(expression ast.Expr) int {
	expression = ps2110Unparen(expression)
	if object := ps6114ExprObject(state.pass, expression); object != nil {
		return state.entries[object]
	}
	if literal, ok := expression.(*ast.BasicLit); ok && literal.Kind == token.STRING && strings.Trim(literal.Value, "`\"") == state.contract.DirectoryName {
		return 1
	}
	if call, ok := expression.(*ast.CallExpr); ok && ps6127CallID(state.pass, call) == "path/filepath.Clean" && len(call.Args) == 1 {
		return state.entryExpr(call.Args[0])
	}
	return 0
}

func ps6127RootMatcher(pass *analysis.Pass, functions map[string]*ast.FuncDecl, function *ast.FuncDecl, contract config.RecursiveMetadataIgnoreContract, rootField, ignoreField *types.Var) bool {
	signature, _ := pass.TypesInfo.Defs[function.Name].Type().(*types.Signature)
	if signature == nil || signature.Results().Len() == 0 || !types.Identical(signature.Results().At(signature.Results().Len()-1).Type(), types.Typ[types.Bool]) {
		return false
	}
	state := &ps6127MatcherState{pass: pass, functions: functions, contract: contract, rootField: rootField, ignoreField: ignoreField, receiverStable: true, inputStable: true, storageStable: true, aliases: make(map[types.Object]types.Object), pointers: make(map[types.Object]types.Object), closures: make(map[types.Object]*ast.FuncLit), bound: make(map[types.Object]bool), flow: ps6122NewFlow(pass, function.Body)}
	if function.Type.Params == nil {
		return false
	}
	if function.Recv != nil && len(function.Recv.List) == 1 && len(function.Recv.List[0].Names) == 1 && len(function.Type.Params.List) > 0 && len(function.Type.Params.List[0].Names) == 1 {
		state.receiver = pass.TypesInfo.ObjectOf(function.Recv.List[0].Names[0])
		state.input = pass.TypesInfo.ObjectOf(function.Type.Params.List[0].Names[0])
	} else if len(function.Type.Params.List) >= 2 && len(function.Type.Params.List[0].Names) == 1 && len(function.Type.Params.List[1].Names) == 1 {
		state.receiver = pass.TypesInfo.ObjectOf(function.Type.Params.List[0].Names[0])
		state.input = pass.TypesInfo.ObjectOf(function.Type.Params.List[1].Names[0])
	} else {
		return false
	}
	state.walk(function.Body.List, true)
	return state.rootOnly && !state.recursive
}

type ps6127MatcherState struct {
	pass                        *analysis.Pass
	contract                    config.RecursiveMetadataIgnoreContract
	rootField                   *types.Var
	ignoreField                 *types.Var
	functions                   map[string]*ast.FuncDecl
	receiver, input             types.Object
	receiverStable, inputStable bool
	storageStable               bool
	aliases                     map[types.Object]types.Object
	pointers                    map[types.Object]types.Object
	closures                    map[types.Object]*ast.FuncLit
	bound                       map[types.Object]bool
	flow                        *ps6122Flow
	path                        types.Object
	rootOnly                    bool
	recursive                   bool
}

func (state *ps6127MatcherState) walk(statements []ast.Stmt, reachable bool) bool {
	for _, statement := range statements {
		if !reachable {
			return true
		}
		switch value := statement.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				object := ps6114ExprObject(state.pass, lhs)
				if object == state.receiver {
					state.receiverStable = false
				}
				if object == state.input {
					state.inputStable = false
				}
			}
			if len(value.Lhs) == 1 && len(value.Rhs) == 1 {
				lhs, rhs := ps6114ExprObject(state.pass, value.Lhs[0]), ps6114ExprObject(state.pass, value.Rhs[0])
				delete(state.closures, lhs)
				delete(state.bound, lhs)
				if literal, ok := ps2110Unparen(value.Rhs[0]).(*ast.FuncLit); ok {
					state.closures[lhs] = literal
				} else if source := ps6114ExprObject(state.pass, value.Rhs[0]); state.closures[source] != nil {
					state.closures[lhs] = state.closures[source]
				}
				if selector, ok := ps2110Unparen(value.Rhs[0]).(*ast.SelectorExpr); ok {
					object := ps6114ExprObject(state.pass, selector.X)
					state.bound[lhs] = object == state.receiver || state.aliases[object] == state.receiver
				} else if source := ps6114ExprObject(state.pass, value.Rhs[0]); state.bound[source] {
					state.bound[lhs] = true
				}
				if unary, ok := ps2110Unparen(value.Rhs[0]).(*ast.UnaryExpr); ok && unary.Op == token.AND {
					state.pointers[lhs] = ps6114ExprObject(state.pass, unary.X)
				} else {
					delete(state.pointers, lhs)
				}
				if rhs == state.receiver || state.aliases[rhs] == state.receiver {
					state.aliases[lhs] = state.receiver
				} else {
					delete(state.aliases, lhs)
				}
			}
			for _, lhs := range value.Lhs {
				state.matcherWrite(lhs)
			}
			for _, rhs := range value.Rhs {
				state.matcherCalls(rhs)
			}
			if len(value.Lhs) == 1 {
				object := ps6114ExprObject(state.pass, value.Lhs[0])
				if object == state.path {
					state.path = nil
				}
				if len(value.Rhs) == 1 && state.pathExpression(value.Rhs[0]) {
					state.path = object
				}
			}
			if len(value.Lhs) != 1 || len(value.Rhs) != 1 {
				for _, lhs := range value.Lhs {
					if ps6114ExprObject(state.pass, lhs) == state.path {
						state.path = nil
					}
				}
			}
		case *ast.RangeStmt:
			selector, _ := ps2110Unparen(value.X).(*ast.SelectorExpr)
			if state.receiverStable && state.storageStable && selector != nil && ps6114ExprObject(state.pass, selector.X) == state.receiver && ps6127FieldObject(state.pass, value.X, state.contract.IgnoreField) == state.ignoreField {
				state.walkRange(value, ps6114ExprObject(state.pass, value.Value))
			} else if state.recursiveRange(value) {
				state.recursive = true
			}
		case *ast.IfStmt:
			if value.Init != nil {
				state.walk([]ast.Stmt{value.Init}, true)
			}
			state.matcherCalls(value.Cond)
			condition := state.pass.TypesInfo.Types[value.Cond].Value
			if condition == nil || constant.BoolVal(condition) {
				if state.walk(value.Body.List, true) && condition != nil {
					return true
				}
			}
			if (condition == nil || !constant.BoolVal(condition)) && value.Else != nil {
				if block, ok := value.Else.(*ast.BlockStmt); ok {
					if state.walk(block.List, true) && condition != nil {
						return true
					}
				}
			}
		case *ast.BlockStmt:
			if state.walk(value.List, true) {
				return true
			}
		case *ast.ReturnStmt:
			if len(value.Results) != 0 {
				result := value.Results[len(value.Results)-1]
				if constantValue := state.pass.TypesInfo.Types[result].Value; constantValue != nil && constantValue.Kind() == constant.Bool && constant.BoolVal(constantValue) {
					state.recursive = true
				}
				if call, ok := ps2110Unparen(result).(*ast.CallExpr); ok && state.recursiveHelper(call) {
					state.recursive = true
				}
			}
			return true
		case *ast.DeferStmt:
			if literal, ok := ps2110Unparen(value.Call.Fun).(*ast.FuncLit); ok && len(literal.Body.List) != 0 {
				state.rootOnly = false
				state.storageStable = false
			}
			state.matcherCalls(value.Call)
			if state.closures[ps6114ExprObject(state.pass, value.Call.Fun)] != nil {
				state.rootOnly = false
				state.storageStable = false
			}
		case *ast.ExprStmt:
			if call, ok := ps2110Unparen(value.X).(*ast.CallExpr); ok {
				if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
					recursive := state.recursive
					state.walk(literal.Body.List, true)
					state.recursive = recursive
				} else if literal := state.closures[ps6114ExprObject(state.pass, call.Fun)]; literal != nil {
					recursive := state.recursive
					state.walk(literal.Body.List, true)
					state.recursive = recursive
				} else if state.bound[ps6114ExprObject(state.pass, call.Fun)] {
					state.storageStable = false
				}
			}
			state.matcherCalls(value.X)
		case *ast.SwitchStmt:
			if value.Init != nil {
				state.walk([]ast.Stmt{value.Init}, true)
			}
			for _, raw := range value.Body.List {
				clause := raw.(*ast.CaseClause)
				if len(clause.List) == 0 || ps6127CaseMayRun(state.pass, value.Tag, clause.List) {
					if state.walk(clause.Body, true) {
						return true
					}
					break
				}
			}
		case *ast.SelectStmt:
			for _, raw := range value.Body.List {
				clause := raw.(*ast.CommClause)
				if clause.Comm == nil && state.walk(clause.Body, true) {
					return true
				}
			}
		case *ast.TypeSwitchStmt:
			if value.Init != nil {
				state.walk([]ast.Stmt{value.Init}, true)
			}
			for _, raw := range value.Body.List {
				if state.walk(raw.(*ast.CaseClause).Body, true) {
					return true
				}
			}
		case *ast.LabeledStmt:
			state.walk([]ast.Stmt{value.Stmt}, true)
		case *ast.DeclStmt:
			declaration, ok := value.Decl.(*ast.GenDecl)
			if ok {
				for _, spec := range declaration.Specs {
					values, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range values.Names {
						if i < len(values.Values) {
							state.walk([]ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{name}, Rhs: []ast.Expr{values.Values[i]}, Tok: token.DEFINE}}, true)
						}
					}
				}
			}
		}
	}
	return false
}

func (state *ps6127MatcherState) walkRange(loop *ast.RangeStmt, entry types.Object) {
	if entry == nil || state.path == nil {
		return
	}
	path, stableEntry := state.path, true
	for _, statement := range loop.Body.List {
		if _, ok := statement.(*ast.ReturnStmt); ok {
			return
		}
		if _, ok := statement.(*ast.BranchStmt); ok {
			return
		}
		if assignment, ok := statement.(*ast.AssignStmt); ok {
			if len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 {
				lhs := ps6114ExprObject(state.pass, assignment.Lhs[0])
				if unary, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.UnaryExpr); ok && unary.Op == token.AND {
					state.pointers[lhs] = ps6114ExprObject(state.pass, unary.X)
				}
				if star, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.StarExpr); ok && state.pointers[ps6114ExprObject(state.pass, star.X)] == path {
					path = nil
				}
				if star, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.StarExpr); ok && state.pointers[ps6114ExprObject(state.pass, star.X)] == entry {
					stableEntry = false
				}
				if literal, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.FuncLit); ok {
					state.closures[ps6114ExprObject(state.pass, assignment.Lhs[0])] = literal
				}
			}
			for _, lhs := range assignment.Lhs {
				object := ps6114ExprObject(state.pass, lhs)
				if object == entry {
					stableEntry = false
				}
				if object == path {
					path = nil
				}
			}
		}
		ast.Inspect(statement, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if literal := state.closures[ps6114ExprObject(state.pass, call.Fun)]; literal != nil {
				ast.Inspect(literal.Body, func(n ast.Node) bool {
					assignment, ok := n.(*ast.AssignStmt)
					if ok {
						for _, lhs := range assignment.Lhs {
							if ps6114ExprObject(state.pass, lhs) == path {
								path = nil
							}
							if ps6114ExprObject(state.pass, lhs) == entry {
								stableEntry = false
							}
						}
					}
					return true
				})
			}
			for _, arg := range call.Args {
				if unary, ok := ps2110Unparen(arg).(*ast.UnaryExpr); ok && unary.Op == token.AND {
					object := ps6114ExprObject(state.pass, unary.X)
					if object == entry {
						stableEntry = false
					}
					if object == path {
						path = nil
					}
				}
			}
			return true
		})
		branch, ok := statement.(*ast.IfStmt)
		if !ok {
			if block, ok := statement.(*ast.BlockStmt); ok {
				state.mutations(block.List, entry, &path, &stableEntry)
			}
			continue
		}
		if branch.Init != nil {
			state.mutations([]ast.Stmt{branch.Init}, entry, &path, &stableEntry)
		}
		condition := state.pass.TypesInfo.Types[branch.Cond].Value
		if condition != nil && !constant.BoolVal(condition) {
			if branch.Else != nil {
				if block, ok := branch.Else.(*ast.BlockStmt); ok {
					state.mutations(block.List, entry, &path, &stableEntry)
				}
			}
			continue
		}
		if stableEntry && path != nil && state.storageStable && ps6122CanEnter(state.pass, state.flow.body, state.flow.parents, branch.Pos()) && ps6127RootPredicate(state.pass, branch.Cond, path, entry) && ps6127ReturnsTrue(state.pass, branch.Body.List) {
			state.rootOnly = true
			continue
		}
		if condition == nil || constant.BoolVal(condition) {
			state.mutations(branch.Body.List, entry, &path, &stableEntry)
		}
	}
}

func (state *ps6127MatcherState) matcherWrite(expression ast.Expr) {
	expression = ps2110Unparen(expression)
	if star, ok := expression.(*ast.StarExpr); ok {
		if state.pointers[ps6114ExprObject(state.pass, star.X)] == state.path {
			state.path = nil
		}
		state.matcherWrite(star.X)
		state.path = nil
		return
	}
	selector := ps6127Selector(expression)
	if selector == nil {
		return
	}
	base := ps6114ExprObject(state.pass, selector.X)
	if base == state.receiver || state.aliases[base] == state.receiver {
		if ps6127FieldObject(state.pass, expression, state.contract.IgnoreField) == state.ignoreField {
			state.storageStable = false
		}
	}
}

func (state *ps6127MatcherState) matcherCalls(expression ast.Expr) {
	ast.Inspect(expression, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id := ps6127CallID(state.pass, call); strings.HasPrefix(id, "path/filepath.") || strings.HasPrefix(id, "strings.") {
			return true
		}
		if literal := state.closures[ps6114ExprObject(state.pass, call.Fun)]; literal != nil {
			recursive := state.recursive
			state.walk(literal.Body.List, true)
			state.recursive = recursive
		}
		if state.bound[ps6114ExprObject(state.pass, call.Fun)] {
			state.storageStable = false
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			object := ps6114ExprObject(state.pass, selector.X)
			if object == state.receiver || state.aliases[object] == state.receiver {
				state.storageStable = false
			}
		}
		for _, arg := range call.Args {
			expr := ps2110Unparen(arg)
			if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
				expr = ps2110Unparen(unary.X)
			}
			if selector := ps6127Selector(expr); selector != nil {
				base := ps6114ExprObject(state.pass, selector.X)
				if base == state.receiver || state.aliases[base] == state.receiver {
					state.storageStable = false
				}
			}
			object := ps6114ExprObject(state.pass, expr)
			if object == state.receiver || state.aliases[object] == state.receiver {
				state.storageStable = false
			}
		}
		return true
	})
}

func (state *ps6127MatcherState) mutations(statements []ast.Stmt, entry types.Object, path *types.Object, stableEntry *bool) {
	for _, statement := range statements {
		switch value := statement.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				object := ps6114ExprObject(state.pass, lhs)
				if object == entry {
					*stableEntry = false
				}
				if object == *path {
					*path = nil
				}
			}
		case *ast.IfStmt:
			if value.Init != nil {
				state.mutations([]ast.Stmt{value.Init}, entry, path, stableEntry)
			}
			condition := state.pass.TypesInfo.Types[value.Cond].Value
			if condition == nil || constant.BoolVal(condition) {
				state.mutations(value.Body.List, entry, path, stableEntry)
			} else if value.Else != nil {
				if block, ok := value.Else.(*ast.BlockStmt); ok {
					state.mutations(block.List, entry, path, stableEntry)
				}
			}
		case *ast.BlockStmt:
			state.mutations(value.List, entry, path, stableEntry)
		}
	}
}

func (state *ps6127MatcherState) pathExpression(expression ast.Expr) bool {
	if !state.receiverStable || !state.inputStable {
		return false
	}
	clean, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || ps6127CallID(state.pass, clean) != "path/filepath.Clean" || len(clean.Args) != 1 {
		return false
	}
	join, ok := ps2110Unparen(clean.Args[0]).(*ast.CallExpr)
	if !ok || ps6127CallID(state.pass, join) != "path/filepath.Join" || len(join.Args) != 2 || ps6127FieldObject(state.pass, join.Args[0], state.contract.RootField) != state.rootField {
		return false
	}
	rootSelector := ps2110Unparen(join.Args[0]).(*ast.SelectorExpr)
	if ps6114ExprObject(state.pass, rootSelector.X) != state.receiver {
		return false
	}
	fromSlash, ok := ps2110Unparen(join.Args[1]).(*ast.CallExpr)
	return ok && ps6127CallID(state.pass, fromSlash) == "path/filepath.FromSlash" && len(fromSlash.Args) == 1 && ps6114ExprObject(state.pass, fromSlash.Args[0]) == state.input
}

func ps6127RootPredicate(pass *analysis.Pass, expression ast.Expr, path, entry types.Object) bool {
	or, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || or.Op != token.LOR {
		return false
	}
	equal, ok := ps2110Unparen(or.X).(*ast.BinaryExpr)
	prefix, prefixOK := ps2110Unparen(or.Y).(*ast.CallExpr)
	if !ok || equal.Op != token.EQL || !prefixOK || ps6127CallID(pass, prefix) != "strings.HasPrefix" || len(prefix.Args) != 2 {
		return false
	}
	left, right := ps6114ExprObject(pass, equal.X), ps6114ExprObject(pass, equal.Y)
	if !((left == path && right == entry) || (left == entry && right == path)) || ps6114ExprObject(pass, prefix.Args[0]) != path {
		return false
	}
	addition, ok := ps2110Unparen(prefix.Args[1]).(*ast.BinaryExpr)
	if !ok || addition.Op != token.ADD || ps6114ExprObject(pass, addition.X) != entry {
		return false
	}
	conversion, ok := ps2110Unparen(addition.Y).(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 {
		return false
	}
	builtin, ok := conversion.Fun.(*ast.Ident)
	selector, selectorOK := ps2110Unparen(conversion.Args[0]).(*ast.SelectorExpr)
	separator, separatorOK := func() (*types.Const, bool) {
		if !selectorOK {
			return nil, false
		}
		object, ok := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Const)
		return object, ok
	}()
	return ok && builtin.Name == "string" && pass.TypesInfo.Uses[builtin] == types.Universe.Lookup("string") && separatorOK && separator.Pkg() != nil && separator.Pkg().Path() == "path/filepath" && separator.Name() == "Separator"
}

func ps6127ReturnsTrue(pass *analysis.Pass, statements []ast.Stmt) bool {
	if len(statements) == 0 {
		return false
	}
	returned, ok := statements[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) == 0 {
		return false
	}
	result := returned.Results[len(returned.Results)-1]
	return pass.TypesInfo.Types[result].Value != nil && constant.BoolVal(pass.TypesInfo.Types[result].Value)
}

func (state *ps6127MatcherState) recursiveRange(loop *ast.RangeStmt) bool {
	call, ok := ps2110Unparen(loop.X).(*ast.CallExpr)
	if !ok || ps6127CallID(state.pass, call) != "strings.Split" || len(call.Args) != 2 {
		return false
	}
	separator := state.pass.TypesInfo.Types[call.Args[1]].Value
	if separator == nil || constant.StringVal(separator) != "/" {
		return false
	}
	toSlash, ok := ps2110Unparen(call.Args[0]).(*ast.CallExpr)
	if !ok || ps6127CallID(state.pass, toSlash) != "path/filepath.ToSlash" || len(toSlash.Args) != 1 || ps6114ExprObject(state.pass, toSlash.Args[0]) != state.input {
		return false
	}
	component := ps6114ExprObject(state.pass, loop.Value)
	for _, statement := range loop.Body.List {
		if _, ok := statement.(*ast.BranchStmt); ok {
			return false
		}
		if _, ok := statement.(*ast.ReturnStmt); ok {
			return false
		}
		if assignment, ok := statement.(*ast.AssignStmt); ok {
			for _, lhs := range assignment.Lhs {
				if ps6114ExprObject(state.pass, lhs) == component {
					return false
				}
			}
		}
		branch, ok := statement.(*ast.IfStmt)
		if !ok || !ps6122CanEnter(state.pass, state.flow.body, state.flow.parents, branch.Pos()) || !ps6127ReturnsTrue(state.pass, branch.Body.List) {
			continue
		}
		equal, ok := ps2110Unparen(branch.Cond).(*ast.BinaryExpr)
		if !ok || equal.Op != token.EQL {
			continue
		}
		var literal ast.Expr
		if ps6114ExprObject(state.pass, equal.X) == component {
			literal = equal.Y
		} else if ps6114ExprObject(state.pass, equal.Y) == component {
			literal = equal.X
		}
		value := state.pass.TypesInfo.Types[literal].Value
		if value != nil && constant.StringVal(value) == state.contract.DirectoryName {
			return true
		}
	}
	return false
}

func (state *ps6127MatcherState) recursiveHelper(call *ast.CallExpr) bool {
	function := state.functions[ps6127CallID(state.pass, call)]
	if function == nil || len(call.Args) != 1 || ps6114ExprObject(state.pass, call.Args[0]) != state.input || function.Type.Params == nil || len(function.Type.Params.List) != 1 || len(function.Type.Params.List[0].Names) != 1 {
		return false
	}
	child := &ps6127MatcherState{pass: state.pass, functions: state.functions, contract: state.contract, input: state.pass.TypesInfo.ObjectOf(function.Type.Params.List[0].Names[0]), flow: ps6122NewFlow(state.pass, function.Body)}
	found := false
	for _, statement := range function.Body.List {
		if _, ok := statement.(*ast.ReturnStmt); ok {
			break
		}
		if loop, ok := statement.(*ast.RangeStmt); ok && child.recursiveRange(loop) {
			found = true
			break
		}
	}
	return found
}

func ps6127CallID(pass *analysis.Pass, call *ast.CallExpr) string {
	function := ps6071CalledFunction(pass, call)
	if function == nil {
		return ""
	}
	return ps6090FunctionID(function)
}

func ps6127FieldObject(pass *analysis.Pass, expression ast.Expr, name string) *types.Var {
	selector, ok := ps2110Unparen(expression).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return nil
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return nil
	}
	field, ok := selection.Obj().(*types.Var)
	if !ok || !field.IsField() {
		return nil
	}
	return field
}

func ps6127AnyFieldObject(pass *analysis.Pass, expression ast.Expr) *types.Var {
	selector := ps6127Selector(expression)
	if selector == nil {
		return nil
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return nil
	}
	field, _ := selection.Obj().(*types.Var)
	if field == nil || !field.IsField() {
		return nil
	}
	return field
}

func ps6127Selector(expression ast.Expr) *ast.SelectorExpr {
	selector, _ := ps2110Unparen(expression).(*ast.SelectorExpr)
	return selector
}

func ps6127Method(pass *analysis.Pass, call *ast.CallExpr, pkgPath, receiverName, methodName string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != methodName {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return false
	}
	function, ok := selection.Obj().(*types.Func)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != pkgPath {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return false
	}
	named, _ := types.Unalias(signature.Recv().Type()).(*types.Pointer)
	if named != nil {
		if n, ok := types.Unalias(named.Elem()).(*types.Named); ok {
			return n.Obj().Name() == receiverName
		}
	}
	if n, ok := types.Unalias(signature.Recv().Type()).(*types.Named); ok {
		return n.Obj().Name() == receiverName
	}
	return false
}
