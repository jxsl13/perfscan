package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"math"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"
)

// ps6084GroupedMatch is the issue #839 extension. It is intentionally
// separate from ps6084Match so the original single-store grammar and its
// diagnostics remain unchanged.
type ps6084GroupedMatch struct {
	loop      ast.Stmt
	scratches []*types.Var
	leaf      *types.Func
	sources   map[*types.Var]types.Object
}

type ps6084GroupedFill struct {
	loop        ast.Stmt
	index       *types.Var
	iterations  int64
	scratches   []*types.Var
	allowed     map[*types.Var]map[*ast.Ident]bool
	sources     map[*types.Var]map[types.Object]bool
	definitions map[types.Object]ps6084GroupedDefinition
}

type ps6084GroupedDefinition struct {
	expression ast.Expr
	before     map[types.Object]ps6084GroupedDefinition
}

func (state *ps6084State) groupedMatch(preprocess, consumer ast.Stmt) (ps6084GroupedMatch, bool) {
	fill, ok := state.groupedFill(preprocess)
	if !ok {
		return ps6084GroupedMatch{}, false
	}
	call, leaf, ok := state.nativeConsumer(consumer)
	if !ok || !state.groupedRepeated(call) {
		return ps6084GroupedMatch{}, false
	}
	signature, ok := leaf.Type().(*types.Signature)
	if !ok || signature.Variadic() || signature.Params().Len() != len(call.Args) {
		return ps6084GroupedMatch{}, false
	}
	matched := make(map[*types.Var]types.Object, len(fill.scratches))
	usedArgs := make([]bool, len(call.Args))
	for _, scratch := range fill.scratches {
		uses, argument, found := ps6084ConsumerScratch(state.pass, call, scratch)
		if !found || argument < 0 || argument >= len(usedArgs) || usedArgs[argument] {
			return ps6084GroupedMatch{}, false
		}
		if parameter := signature.Params().At(argument).Type(); !types.Identical(parameter, state.pass.TypesInfo.TypeOf(call.Args[argument])) {
			return ps6084GroupedMatch{}, false
		}
		usedArgs[argument] = true
		for identifier := range uses {
			fill.allowed[scratch][identifier] = true
		}
		source, _ := state.groupedConsumerSource(call, signature, argument, fill.sources[scratch], fill.definitions)
		if source == nil || !ps6084OnlyScratchUses(state.pass, state.function.Body, scratch, fill.allowed[scratch]) {
			return ps6084GroupedMatch{}, false
		}
		matched[scratch] = source
	}
	if len(matched) != len(fill.scratches) {
		return ps6084GroupedMatch{}, false
	}
	return ps6084GroupedMatch{loop: fill.loop, scratches: fill.scratches, leaf: leaf, sources: matched}, true
}

func (state *ps6084State) groupedConsumerSource(call *ast.CallExpr, signature *types.Signature, scratchArgument int, references map[types.Object]bool, definitions map[types.Object]ps6084GroupedDefinition) (types.Object, bool) {
	var matched types.Object
	for argumentIndex, expression := range call.Args {
		if argumentIndex == scratchArgument || !types.Identical(signature.Params().At(argumentIndex).Type(), state.pass.TypesInfo.TypeOf(expression)) {
			continue
		}
		candidate, offset, ok := state.groupedSourceCarrier(expression)
		if !ok {
			continue
		}
		boundary, hasBoundary := int64(0), false
		all := true
		for reference := range references {
			low, high, bounded, derived := state.groupedDerivedInterval(reference, candidate, definitions, map[types.Object]bool{})
			if !derived || low < 0 {
				all = false
				break
			}
			if bounded && high > boundary {
				boundary, hasBoundary = high, true
			}
		}
		if !all || offset != 0 && (!hasBoundary || offset != boundary) {
			continue
		}
		if matched != nil && state.aliasFind(matched) != state.aliasFind(candidate) {
			return nil, false
		}
		matched = candidate
	}
	return matched, matched != nil
}

func (state *ps6084State) groupedSourceCarrier(expression ast.Expr) (types.Object, int64, bool) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := state.pass.TypesInfo.ObjectOf(value)
		return object, 0, object != nil && ps6084PackedType(object.Type())
	case *ast.UnaryExpr:
		if value.Op != token.AND {
			return nil, 0, false
		}
		if indexed, ok := ps2110Unparen(value.X).(*ast.IndexExpr); ok {
			offset := ps6084ConstantInt(state.pass, indexed.Index)
			identifier, identifierOK := ps2110Unparen(indexed.X).(*ast.Ident)
			if !identifierOK {
				return nil, 0, false
			}
			object := state.pass.TypesInfo.ObjectOf(identifier)
			if offset < 0 || object == nil || !ps6084PackedType(object.Type()) {
				return nil, 0, false
			}
			return object, offset, true
		}
		return state.groupedSourceCarrier(value.X)
	case *ast.IndexExpr:
		// Indexing copies one element; without an address operation it is not
		// evidence that the native boundary receives the packed aggregate.
		return nil, 0, false
	case *ast.CallExpr:
		if !ps6084CarrierPreservingConversion(state.pass, value) {
			return nil, 0, false
		}
		return state.groupedSourceCarrier(value.Args[0])
	default:
		return nil, 0, false
	}
}

func (state *ps6084State) groupedDerivedInterval(reference, candidate types.Object, definitions map[types.Object]ps6084GroupedDefinition, visiting map[types.Object]bool) (int64, int64, bool, bool) {
	if reference == nil || candidate == nil || visiting[reference] {
		return 0, 0, false, false
	}
	if state.aliasFind(reference) == state.aliasFind(candidate) {
		return 0, 0, false, true
	}
	if state.writes[reference] != state.definitions[reference] {
		return 0, 0, false, false
	}
	definition, found := definitions[reference]
	if !found {
		return 0, 0, false, false
	}
	slice, ok := ps2110Unparen(definition.expression).(*ast.SliceExpr)
	if !ok || slice.Slice3 || slice.High == nil {
		return 0, 0, false, false
	}
	baseIdentifier, ok := ps2110Unparen(slice.X).(*ast.Ident)
	if !ok {
		return 0, 0, false, false
	}
	base := state.pass.TypesInfo.ObjectOf(baseIdentifier)
	low, high := int64(0), ps6084ConstantInt(state.pass, slice.High)
	if slice.Low != nil {
		low = ps6084ConstantInt(state.pass, slice.Low)
	}
	if base == nil || low < 0 || high < low {
		return 0, 0, false, false
	}
	visiting[reference] = true
	baseLow, _, _, derived := state.groupedDerivedInterval(base, candidate, definition.before, visiting)
	delete(visiting, reference)
	if !derived {
		return 0, 0, false, false
	}
	return baseLow + low, baseLow + high, true, true
}

func (state *ps6084State) groupedFill(statement ast.Stmt) (ps6084GroupedFill, bool) {
	var body *ast.BlockStmt
	var index *types.Var
	var induction []int64
	switch loop := statement.(type) {
	case *ast.RangeStmt:
		body = loop.Body
		if loop.Tok != token.DEFINE || loop.Value != nil {
			return ps6084GroupedFill{}, false
		}
		identifier, ok := ps2110Unparen(loop.Key).(*ast.Ident)
		if !ok {
			return ps6084GroupedFill{}, false
		}
		index, _ = state.pass.TypesInfo.Defs[identifier].(*types.Var)
		count, known := ps6099RangeExactIterations(state.pass, loop.X)
		if !known || count < 2 || count > uint64(ps6084MaximumScratch) || count > math.MaxInt64 {
			return ps6084GroupedFill{}, false
		}
		for value := int64(0); value < int64(count); value++ {
			induction = append(induction, value)
		}
	case *ast.ForStmt:
		body = loop.Body
		count, known := ps6099ForExactIterations(state.pass, loop)
		if !known || count < 2 || count > uint64(ps6084MaximumScratch) || count > math.MaxInt64 {
			return ps6084GroupedFill{}, false
		}
		assignment, ok := loop.Init.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 {
			return ps6084GroupedFill{}, false
		}
		identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !ok {
			return ps6084GroupedFill{}, false
		}
		index, _ = state.pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		start, step, ok := state.groupedForSequence(loop, index)
		if !ok {
			return ps6084GroupedFill{}, false
		}
		value := start
		for range count {
			induction = append(induction, value)
			next, valid := state.groupedCheckedAdd(value, step)
			if !valid {
				return ps6084GroupedFill{}, false
			}
			value = next
		}
	default:
		return ps6084GroupedFill{}, false
	}
	if index == nil || len(body.List) < 2 || len(body.List) > 16 {
		return ps6084GroupedFill{}, false
	}

	definitions := state.groupedDefinitionsBefore(statement.Pos())
	type write struct {
		scratch *types.Var
		lhs     *ast.Ident
		index   ast.Expr
		rhs     ast.Expr
		defs    map[types.Object]ps6084GroupedDefinition
	}
	var writes []write
	for _, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != len(assignment.Rhs) || assignment.Tok != token.DEFINE && assignment.Tok != token.ASSIGN {
			return ps6084GroupedFill{}, false
		}
		for _, expression := range assignment.Rhs {
			if state.groupedExpressionHasEffects(expression) {
				return ps6084GroupedFill{}, false
			}
		}
		var pendingObjects []types.Object
		var pendingValues []ps6084GroupedDefinition
		for position, left := range assignment.Lhs {
			indexed, indexedOK := ps2110Unparen(left).(*ast.IndexExpr)
			if indexedOK {
				if assignment.Tok != token.ASSIGN {
					return ps6084GroupedFill{}, false
				}
				identifier, identifierOK := ps2110Unparen(indexed.X).(*ast.Ident)
				scratch, scratchOK := state.pass.TypesInfo.ObjectOf(identifier).(*types.Var)
				if !identifierOK || !scratchOK || scratch.IsField() || scratch.Parent() == state.pass.Pkg.Scope() {
					return ps6084GroupedFill{}, false
				}
				array, arrayOK := types.Unalias(scratch.Type()).Underlying().(*types.Array)
				if !arrayOK || array.Len() < ps6084MinimumScratch || array.Len() > ps6084MaximumScratch || !ps6084NumericType(array.Elem()) {
					return ps6084GroupedFill{}, false
				}
				writes = append(writes, write{scratch: scratch, lhs: identifier, index: indexed.Index, rhs: assignment.Rhs[position], defs: maps.Clone(definitions)})
				continue
			}
			identifier, identifierOK := ps2110Unparen(left).(*ast.Ident)
			if !identifierOK || assignment.Tok != token.DEFINE {
				return ps6084GroupedFill{}, false
			}
			object := state.pass.TypesInfo.ObjectOf(identifier)
			variable, variableOK := object.(*types.Var)
			if object == nil || object == index || !variableOK || variable.IsField() || variable.Parent() == state.pass.Pkg.Scope() {
				return ps6084GroupedFill{}, false
			}
			pendingObjects = append(pendingObjects, object)
			pendingValues = append(pendingValues, ps6084GroupedDefinition{expression: assignment.Rhs[position], before: maps.Clone(definitions)})
		}
		// Go evaluates every RHS before assigning any LHS. Publish the new
		// versions together so a later store cannot rewrite an earlier snapshot.
		for position, object := range pendingObjects {
			definitions[object] = pendingValues[position]
		}
	}
	if len(writes) < 2 || len(writes) > 16 {
		return ps6084GroupedFill{}, false
	}

	type scratchCoverage struct {
		covered []bool
		count   int
	}
	coverage := make(map[*types.Var]*scratchCoverage)
	allowed := make(map[*types.Var]map[*ast.Ident]bool)
	sources := make(map[*types.Var]map[types.Object]bool)
	for _, item := range writes {
		array := types.Unalias(item.scratch.Type()).Underlying().(*types.Array)
		if coverage[item.scratch] == nil {
			coverage[item.scratch] = &scratchCoverage{covered: make([]bool, int(array.Len()))}
			allowed[item.scratch] = make(map[*ast.Ident]bool)
			sources[item.scratch] = make(map[types.Object]bool)
		}
		allowed[item.scratch][item.lhs] = true
		for _, iteration := range induction {
			value, ok := state.groupedInteger(item.index, index, iteration, item.defs, map[types.Object]bool{})
			if !ok || value < 0 || value >= array.Len() || coverage[item.scratch].covered[int(value)] {
				return ps6084GroupedFill{}, false
			}
			coverage[item.scratch].covered[int(value)] = true
			coverage[item.scratch].count++
		}
		roots, ok := state.groupedPackedRoots(item.rhs, index, item.defs, map[types.Object]bool{})
		if !ok || len(roots) == 0 {
			return ps6084GroupedFill{}, false
		}
		for root := range roots {
			sources[item.scratch][root] = true
		}
	}
	if len(coverage) < 1 || len(coverage) > 2 {
		return ps6084GroupedFill{}, false
	}
	var scratches []*types.Var
	for scratch, indices := range coverage {
		array := types.Unalias(scratch.Type()).Underlying().(*types.Array)
		complete := indices.count == len(indices.covered)
		for _, covered := range indices.covered {
			complete = complete && covered
		}
		if int64(len(indices.covered)) != array.Len() || !complete || !state.zeroInitialized(scratch, statement.Pos()) {
			return ps6084GroupedFill{}, false
		}
		scratches = append(scratches, scratch)
	}
	slices.SortFunc(scratches, func(left, right *types.Var) int { return strings.Compare(left.Name(), right.Name()) })
	return ps6084GroupedFill{loop: statement, index: index, iterations: int64(len(induction)), scratches: scratches, allowed: allowed, sources: sources, definitions: definitions}, true
}

func (state *ps6084State) groupedExpressionHasEffects(expression ast.Expr) bool {
	effect := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if effect {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if state.pass.TypesInfo.Types[call.Fun].IsType() {
			return true
		}
		if _, valid := state.groupedProvenanceCall(call); !valid {
			effect = true
			return false
		}
		return true
	})
	return effect
}

func (state *ps6084State) groupedForSequence(loop *ast.ForStmt, index *types.Var) (int64, int64, bool) {
	assignment, ok := loop.Init.(*ast.AssignStmt)
	if !ok || len(assignment.Rhs) != 1 {
		return 0, 0, false
	}
	start := ps6084ConstantInt(state.pass, assignment.Rhs[0])
	if start < 0 {
		return 0, 0, false
	}
	switch post := loop.Post.(type) {
	case *ast.IncDecStmt:
		if ps6084IdentifierObject(state.pass, post.X) != index {
			return 0, 0, false
		}
		if post.Tok == token.INC {
			return start, 1, true
		}
		if post.Tok == token.DEC {
			return start, -1, true
		}
	case *ast.AssignStmt:
		if len(post.Lhs) != 1 || len(post.Rhs) != 1 || ps6084IdentifierObject(state.pass, post.Lhs[0]) != index {
			return 0, 0, false
		}
		step := ps6084ConstantInt(state.pass, post.Rhs[0])
		if step <= 0 {
			return 0, 0, false
		}
		if post.Tok == token.ADD_ASSIGN {
			return start, step, true
		}
		if post.Tok == token.SUB_ASSIGN {
			return start, -step, true
		}
	}
	return 0, 0, false
}

func (state *ps6084State) groupedDefinitionsBefore(before token.Pos) map[types.Object]ps6084GroupedDefinition {
	result := make(map[types.Object]ps6084GroupedDefinition)
	ast.Inspect(state.function.Body, func(node ast.Node) bool {
		if node == nil || node.Pos() >= before {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) == len(value.Rhs) {
				beforeAssignment := maps.Clone(result)
				for index, left := range value.Lhs {
					if identifier, ok := ps2110Unparen(left).(*ast.Ident); ok {
						if object := state.pass.TypesInfo.ObjectOf(identifier); object != nil {
							result[object] = ps6084GroupedDefinition{expression: value.Rhs[index], before: beforeAssignment}
						}
					}
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				beforeSpec := maps.Clone(result)
				for index, name := range value.Names {
					if object := state.pass.TypesInfo.Defs[name]; object != nil {
						result[object] = ps6084GroupedDefinition{expression: value.Values[index], before: beforeSpec}
					}
				}
			}
		}
		return true
	})
	return result
}

func (state *ps6084State) groupedInteger(expression ast.Expr, index *types.Var, iteration int64, definitions map[types.Object]ps6084GroupedDefinition, visiting map[types.Object]bool) (int64, bool) {
	expression = ps2110Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		object := state.pass.TypesInfo.ObjectOf(identifier)
		if object == index {
			return iteration, true
		}
		if definition, found := definitions[object]; found && !visiting[object] {
			visiting[object] = true
			value, valid := state.groupedInteger(definition.expression, index, iteration, definition.before, visiting)
			delete(visiting, object)
			return value, valid
		}
	}
	if value := state.pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Int {
		integer, exact := constant.Int64Val(value)
		return integer, exact
	}
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD && binary.Op != token.SUB && binary.Op != token.MUL {
		return 0, false
	}
	left, leftOK := state.groupedInteger(binary.X, index, iteration, definitions, visiting)
	right, rightOK := state.groupedInteger(binary.Y, index, iteration, definitions, visiting)
	if !leftOK || !rightOK {
		return 0, false
	}
	switch binary.Op {
	case token.ADD:
		if right > 0 && left > math.MaxInt64-right || right < 0 && left < math.MinInt64-right {
			return 0, false
		}
		return left + right, true
	case token.SUB:
		if right == math.MinInt64 {
			return 0, false
		}
		return state.groupedCheckedAdd(left, -right)
	case token.MUL:
		if left == 0 || right == 0 {
			return 0, true
		}
		if left == math.MinInt64 && right == -1 || right == math.MinInt64 && left == -1 {
			return 0, false
		}
		value := left * right
		return value, value/right == left
	}
	return 0, false
}

func (state *ps6084State) groupedCheckedAdd(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right || right < 0 && left < math.MinInt64-right {
		return 0, false
	}
	return left + right, true
}

func (state *ps6084State) groupedPackedRoots(expression ast.Expr, index *types.Var, definitions map[types.Object]ps6084GroupedDefinition, visiting map[types.Object]bool) (map[types.Object]bool, bool) {
	result := make(map[types.Object]bool)
	valid := true
	var walk func(ast.Expr)
	walk = func(raw ast.Expr) {
		if !valid || raw == nil {
			return
		}
		expression := ps2110Unparen(raw)
		switch value := expression.(type) {
		case *ast.Ident:
			object := state.pass.TypesInfo.ObjectOf(value)
			if object == nil || object == index {
				return
			}
			if variable, ok := object.(*types.Var); ok && !variable.IsField() && ps6084PackedType(variable.Type()) {
				if _, found := definitions[object]; !found || ps6084PackedContainer(variable.Type()) {
					result[state.aliasFind(variable)] = true
					return
				}
			}
			if definition, found := definitions[object]; found {
				if state.writes[object] != state.definitions[object] {
					valid = false
					return
				}
				if visiting[object] {
					valid = false
					return
				}
				visiting[object] = true
				prior := definitions
				definitions = definition.before
				walk(definition.expression)
				definitions = prior
				delete(visiting, object)
				return
			}
		case *ast.CallExpr:
			if state.pass.TypesInfo.Types[value.Fun].IsType() {
				if len(value.Args) != 1 || !ps6084NumericType(state.pass.TypesInfo.TypeOf(value)) || !ps6084NumericType(state.pass.TypesInfo.TypeOf(value.Args[0])) {
					valid = false
					return
				}
				walk(value.Args[0])
				return
			}
			arguments, ok := state.groupedProvenanceCall(value)
			if !ok {
				valid = false
				return
			}
			for _, argument := range arguments {
				walk(argument)
			}
		case *ast.IndexExpr:
			walk(value.X)
			walk(value.Index)
		case *ast.SliceExpr:
			walk(value.X)
			walk(value.Low)
			walk(value.High)
			walk(value.Max)
		case *ast.SelectorExpr:
			walk(value.X)
		case *ast.BinaryExpr:
			walk(value.X)
			walk(value.Y)
		case *ast.UnaryExpr:
			if value.Op == token.ARROW || value.Op == token.AND {
				valid = false
				return
			}
			walk(value.X)
		case *ast.BasicLit:
		default:
			valid = false
		}
	}
	walk(expression)
	return result, valid
}

func ps6084PackedContainer(value types.Type) bool {
	switch types.Unalias(value).Underlying().(type) {
	case *types.Array, *types.Slice, *types.Pointer:
		return true
	}
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.String
}

func (state *ps6084State) groupedProvenanceCall(call *ast.CallExpr) ([]ast.Expr, bool) {
	function := ps6071CalledFunction(state.pass, call)
	if function == nil || call.Ellipsis.IsValid() {
		return nil, false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Variadic() || signature.Results().Len() != 1 || len(call.Args) != signature.Params().Len() || !ps6084NumericType(signature.Results().At(0).Type()) {
		return nil, false
	}
	if function.Pkg() != nil && function.Pkg().Path() == "encoding/binary" && function.Name() == "Uint16" && signature.Params().Len() == 1 {
		parameter, result := signature.Params().At(0).Type(), signature.Results().At(0).Type()
		slice, sliceOK := types.Unalias(parameter).Underlying().(*types.Slice)
		basic, basicOK := types.Unalias(result).Underlying().(*types.Basic)
		selector, selectorOK := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		if !selectorOK {
			return nil, false
		}
		receiver, receiverOK := ps2110Unparen(selector.X).(*ast.SelectorExpr)
		if !receiverOK {
			return nil, false
		}
		littleEndian, littleEndianOK := state.pass.TypesInfo.ObjectOf(receiver.Sel).(*types.Var)
		if !sliceOK || !ps6084IntegerElement(slice.Elem()) || !basicOK || basic.Kind() != types.Uint16 || !littleEndianOK || littleEndian.Pkg() == nil || littleEndian.Pkg().Path() != "encoding/binary" || littleEndian.Name() != "LittleEndian" {
			return nil, false
		}
		return call.Args, true
	}
	if candidate := state.groupedFunctionDeclarations()[function]; candidate != nil && candidate.Body != nil && len(candidate.Body.List) == 1 {
		returned, ok := candidate.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(returned.Results) != 1 || ps6084ExpressionHasEffects(state.pass, returned.Results[0]) {
			return nil, false
		}
		used := make(map[*types.Var]bool)
		valid := true
		ast.Inspect(returned.Results[0], func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			variable, ok := state.pass.TypesInfo.ObjectOf(identifier).(*types.Var)
			if !ok || variable.IsField() {
				return true
			}
			for index := 0; index < signature.Params().Len(); index++ {
				if variable == signature.Params().At(index) {
					used[variable] = true
					return true
				}
			}
			if variable.Parent() == state.pass.Pkg.Scope() && !state.groupedReadOnlyTable(variable) {
				valid = false
				return false
			}
			return true
		})
		if !valid {
			return nil, false
		}
		var arguments []ast.Expr
		for index := 0; index < signature.Params().Len(); index++ {
			if used[signature.Params().At(index)] {
				arguments = append(arguments, call.Args[index])
			}
		}
		return arguments, len(arguments) != 0
	}
	return nil, false
}

func (state *ps6084State) groupedReadOnlyTable(variable *types.Var) bool {
	if state.groupedTables != nil {
		if result, ok := state.groupedTables[variable]; ok {
			return result
		}
	} else {
		state.groupedTables = make(map[*types.Var]bool)
	}
	array, ok := types.Unalias(variable.Type()).Underlying().(*types.Array)
	if !ok || !ps6084NumericType(array.Elem()) {
		state.groupedTables[variable] = false
		return false
	}
	safe := true
	var borrows []ast.Node
	for _, file := range state.pass.Files {
		parents := ps6071Parents(file)
		ast.Inspect(file, func(node ast.Node) bool {
			if !safe {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if !ok || state.pass.TypesInfo.ObjectOf(identifier) != variable {
				return true
			}
			switch parent := parents[identifier].(type) {
			case *ast.ValueSpec:
				return true
			case *ast.IndexExpr:
				if parent.X == identifier {
					operation := ps6084TableOperation(parents, parent)
					switch value := operation.(type) {
					case *ast.UnaryExpr:
						region, borrowed := state.groupedNoescapeBorrow(parents, value)
						if !borrowed {
							safe = false
						} else {
							borrows = append(borrows, region)
						}
					case *ast.IncDecStmt:
						safe = false
					case *ast.AssignStmt:
						if ps6084AssignmentWritesExpression(value, parent) && !ps6084InitializationWrite(parents, operation) {
							safe = false
						}
					}
				}
			case *ast.SliceExpr:
				// A slice aliases the table's storage. Proving every transitive use is
				// read-only is outside this bounded source contract.
				if parent.X == identifier {
					safe = false
				}
			case *ast.UnaryExpr:
				if parent.Op == token.AND {
					safe = false
				}
			case *ast.AssignStmt, *ast.IncDecStmt:
				safe = false
			}
			return safe
		})
	}
	if !safe || len(borrows) == 0 {
		state.groupedTables[variable] = safe
		return safe
	}
	reachable, unknown := state.groupedReachableRegions()
	if unknown {
		state.groupedTables[variable] = false
		return false
	}
	for _, region := range borrows {
		if reachable[region] {
			state.groupedTables[variable] = false
			return false
		}
	}
	state.groupedTables[variable] = true
	return true
}

func (state *ps6084State) groupedNoescapeBorrow(parents map[ast.Node]ast.Node, address *ast.UnaryExpr) (ast.Node, bool) {
	var call *ast.CallExpr
	for current := ast.Node(address); current != nil; current = parents[current] {
		switch parent := parents[current].(type) {
		case *ast.ParenExpr:
			continue
		case *ast.CallExpr:
			call = parent
		default:
			return nil, false
		}
		break
	}
	if call == nil {
		return nil, false
	}
	function := ps6071CalledFunction(state.pass, call)
	if function == nil || state.leaves[function] == nil {
		return nil, false
	}
	for current := ast.Node(call); current != nil; current = parents[current] {
		switch region := current.(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			return nil, false
		case *ast.FuncLit:
			return region, true
		case *ast.FuncDecl:
			return region, true
		}
	}
	return nil, false
}

func (state *ps6084State) groupedReachableRegions() (map[ast.Node]bool, bool) {
	if state.groupedRegionsReady {
		return state.groupedRegions, state.groupedRegionsUnknown
	}
	declarations := state.groupedFunctionDeclarations()
	reachable := map[ast.Node]bool{}
	visiting := map[*ast.FuncDecl]bool{}
	unknown := false
	var walkBody func(*ast.BlockStmt)
	var walkCallback func(ast.Expr)
	walkFunction := func(function *ast.FuncDecl) {
		if function == nil || function.Body == nil || visiting[function] {
			return
		}
		visiting[function] = true
		reachable[function] = true
		walkBody(function.Body)
	}
	walkCallback = func(expression ast.Expr) {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.FuncLit:
			reachable[value] = true
			walkBody(value.Body)
		case *ast.Ident:
			function, ok := state.pass.TypesInfo.ObjectOf(value).(*types.Func)
			if !ok {
				unknown = true
				return
			}
			if declaration := declarations[function]; declaration != nil {
				walkFunction(declaration)
			}
		case *ast.SelectorExpr:
			function, direct := ps6084SelectedFunction(state.pass, value)
			if !direct {
				unknown = true
				return
			}
			if declaration := declarations[function]; declaration != nil {
				walkFunction(declaration)
			}
		default:
			unknown = true
		}
	}
	walkBody = func(body *ast.BlockStmt) {
		ps6122WalkExecuted(body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
				reachable[literal] = true
				return true
			}
			if state.pass.TypesInfo.Types[call.Fun].IsType() {
				return true
			}
			if identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
				if _, builtin := state.pass.TypesInfo.ObjectOf(identifier).(*types.Builtin); builtin {
					return true
				}
			}
			if !ps6084DirectCall(state.pass, call) {
				unknown = true
				return true
			}
			called := ps6071CalledFunction(state.pass, call)
			if called == nil {
				unknown = true
				return true
			}
			declaration := declarations[called]
			if declaration != nil && declaration.Body != nil {
				walkFunction(declaration)
				return true
			}
			for _, argument := range call.Args {
				if ps4008TypeMayCarryCallable(state.pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
					walkCallback(argument)
				}
			}
			return true
		})
	}
	walkFunction(state.function)
	state.groupedRegions, state.groupedRegionsUnknown, state.groupedRegionsReady = reachable, unknown, true
	return reachable, unknown
}

func (state *ps6084State) groupedFunctionDeclarations() map[*types.Func]*ast.FuncDecl {
	if state.groupedDecls != nil {
		return state.groupedDecls
	}
	declarations := make(map[*types.Func]*ast.FuncDecl)
	for _, file := range state.pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if object, ok := state.pass.TypesInfo.ObjectOf(function.Name).(*types.Func); ok {
				declarations[object] = function
			}
		}
	}
	state.groupedDecls = declarations
	return declarations
}

func ps6084DirectCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if pass == nil || pass.TypesInfo == nil || call == nil {
		return false
	}
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return ps6071CalledFunction(pass, call) != nil
	}
	_, direct := ps6084SelectedFunction(pass, selector)
	return direct
}

func ps6084SelectedFunction(pass *analysis.Pass, selector *ast.SelectorExpr) (*types.Func, bool) {
	if pass == nil || pass.TypesInfo == nil || selector == nil {
		return nil, false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		// A qualified package selector is statically bound.
		function, direct := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
		return function, direct
	}
	// A method selected from an interface receiver is dynamically dispatched,
	// even though go/types exposes the selected method as a *types.Func. Inspect
	// both the outer receiver and the method's declared receiver: a concrete
	// struct may promote a dynamically dispatched method from an embedded
	// interface.
	function, direct := selection.Obj().(*types.Func)
	if !direct || ps6084InterfaceType(selection.Recv()) {
		return nil, false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() == nil || ps6084InterfaceType(signature.Recv().Type()) {
		return nil, false
	}
	return function, true
}

func ps6084InterfaceType(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := types.Unalias(value).Underlying().(*types.Interface)
	return ok
}

func ps6084TableOperation(parents map[ast.Node]ast.Node, node ast.Node) ast.Node {
	for current := node; current != nil; current = parents[current] {
		switch parent := parents[current].(type) {
		case *ast.ParenExpr:
			continue
		case *ast.UnaryExpr:
			if parent.Op == token.AND {
				return parent
			}
			return nil
		case *ast.AssignStmt, *ast.IncDecStmt:
			return parent
		default:
			return nil
		}
	}
	return nil
}

func ps6084AssignmentWritesExpression(assignment *ast.AssignStmt, expression ast.Expr) bool {
	if assignment == nil {
		return false
	}
	for _, left := range assignment.Lhs {
		if ps2110Unparen(left) == expression {
			return true
		}
	}
	return false
}

func ps6084InitializationWrite(parents map[ast.Node]ast.Node, node ast.Node) bool {
	for current := node; current != nil; current = parents[current] {
		switch region := current.(type) {
		case *ast.FuncLit:
			call, ok := parents[region].(*ast.CallExpr)
			if !ok || ps2110Unparen(call.Fun) != region {
				return false
			}
			switch parents[call].(type) {
			case *ast.GoStmt, *ast.DeferStmt:
				return false
			}
		case *ast.FuncDecl:
			return region.Recv == nil && region.Name.Name == "init"
		}
	}
	return false
}

func (state *ps6084State) groupedRepeated(node ast.Node) bool {
	flow := state.groupedFlow
	if flow == nil {
		flow = ps6122NewFlow(state.pass, state.function.Body)
		state.groupedFlow = flow
	}
	for parent := state.parents[node]; parent != nil; parent = state.parents[parent] {
		switch loop := parent.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		case *ast.ForStmt:
			iterations, known := ps6099ForExactIterations(state.pass, loop)
			if known && iterations < 2 || !known && !state.groupedPotentialManyFor(loop) {
				continue
			}
			return state.groupedIIFEsComplete(loop.Body) && state.groupedLoopRepeats(flow, node, loop)
		case *ast.RangeStmt:
			// x/tools' range header is not a revisited expression node. Until a
			// label-aware range backedge query exists, keep this extension silent.
			continue
		}
	}
	return false
}

func (state *ps6084State) groupedLoopRepeats(flow *ps6122Flow, node ast.Node, loop *ast.ForStmt) bool {
	start := ps6122BlockAt(state.pass, flow, node.Pos())
	if start == nil || ps6122BlockHasBlockingTransfer(state.pass, flow, start, node.Pos(), token.NoPos) {
		return false
	}
	var backedge *cfg.Block
	for _, block := range flow.graph.Blocks {
		if block.Kind == cfg.KindForLoop && block.Stmt == loop {
			backedge = block
			break
		}
	}
	return backedge != nil && ps6084GroupedFlowPath(state.pass, flow, start, backedge) && ps6084GroupedFlowPath(state.pass, flow, backedge, start)
}

func ps6084GroupedFlowPath(pass *analysis.Pass, flow *ps6122Flow, start, target *cfg.Block) bool {
	if start == nil || target == nil || !flow.reachable[start] || !flow.reachable[target] {
		return false
	}
	seen := map[*cfg.Block]bool{}
	pending := []*cfg.Block{start}
	for len(pending) != 0 {
		block := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if block == target {
			return true
		}
		if block == nil || seen[block] || !flow.reachable[block] {
			continue
		}
		seen[block] = true
		pending = append(pending, ps6122Successors(pass, flow, block, token.NoPos)...)
	}
	return false
}

func (state *ps6084State) groupedIIFEsComplete(body *ast.BlockStmt) bool {
	complete := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !complete {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if ok {
			literal, invoked := ps2110Unparen(call.Fun).(*ast.FuncLit)
			if invoked {
				parents := ps6071Parents(literal.Body)
				if !ps6121FallsThrough(state.pass, literal.Body) && !ps6099AllPathsReturn(state.pass, literal.Body, parents) {
					complete = false
				}
				return false
			}
		}
		_, literal := node.(*ast.FuncLit)
		return !literal
	})
	return complete
}

func (state *ps6084State) groupedPotentialManyFor(loop *ast.ForStmt) bool {
	initialization, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initialization.Tok != token.DEFINE || len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 || ps6084ConstantInt(state.pass, initialization.Rhs[0]) != 0 {
		return false
	}
	identifier, ok := ps2110Unparen(initialization.Lhs[0]).(*ast.Ident)
	if !ok {
		return false
	}
	index, ok := state.pass.TypesInfo.ObjectOf(identifier).(*types.Var)
	basic, basicOK := types.Unalias(index.Type()).Underlying().(*types.Basic)
	post, postOK := loop.Post.(*ast.IncDecStmt)
	condition, conditionOK := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if !ok || !basicOK || basic.Kind() != types.Int || !postOK || post.Tok != token.INC || ps6084IdentifierObject(state.pass, post.X) != index || !conditionOK || condition.Op != token.LSS || state.writes[index] != 2 {
		return false
	}
	if !state.groupedPositiveIndexFactor(condition.X, index) || !state.groupedInvariantBound(condition.Y, index) {
		return false
	}
	return true
}

func (state *ps6084State) groupedPositiveIndexFactor(expression ast.Expr, index *types.Var) bool {
	expression = ps2110Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		return state.pass.TypesInfo.ObjectOf(identifier) == index
	}
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.MUL {
		return false
	}
	if state.groupedPositiveIndexFactor(binary.X, index) {
		return ps6084ConstantInt(state.pass, binary.Y) > 0
	}
	return state.groupedPositiveIndexFactor(binary.Y, index) && ps6084ConstantInt(state.pass, binary.X) > 0
}

func (state *ps6084State) groupedInvariantBound(expression ast.Expr, index *types.Var) bool {
	valid := !ps6084ExpressionHasEffects(state.pass, expression)
	ast.Inspect(expression, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := state.pass.TypesInfo.ObjectOf(identifier)
		if object == index {
			valid = false
			return false
		}
		if variable, ok := object.(*types.Var); ok && state.writes[variable] != state.definitions[variable] {
			valid = false
			return false
		}
		return true
	})
	return valid
}

func ps6084ReportGrouped(pass *analysis.Pass, match *ps6084GroupedMatch) {
	names := make([]string, 0, len(match.scratches))
	sources := make([]string, 0, len(match.scratches))
	for _, scratch := range match.scratches {
		names = append(names, scratch.Name())
		sources = append(sources, match.sources[scratch].Name())
	}
	pass.Reportf(match.loop.Pos(), "fixed scratch arrays %s are completely overwritten by this affine Go fill, used nowhere else, and immediately consumed with paired packed sources %s by noescape native/assembly leaf %s on a path that may repeat; benchmark whether a coarser native row boundary can own the header decode and scratch lifetime (advisory, no automatic fix). Preserve exact dtype and operation order, numerical behavior, alignment, bounds, aliases, tails, ABI, feature gates, and fallbacks; verify emitted frame/zeroing behavior and require same-binary alternating-order complete-operation benchmarks",
		strings.Join(names, ", "), strings.Join(sources, ", "), match.leaf.Name())
}
