package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6105 implements owner issue #884. It finds a fixed constructor-owned
// scratch allocation whose exact field is either never read or passed whole
// only to unused parameters of statically concrete package-local callees.
var PS6105 = register(&lint.Check{
	ID:       "PS6105",
	Category: "alloc",
	Slug:     "dead-projection-scratch-after-accumulate",
	Level:    lint.LevelAggressive,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "constructor-stored projection scratch has no ordinary typed consumer",
		Text: `A projection or linear layer can retain a full output-sized scratch
buffer after its caller has switched to an accumulating epilogue that writes
directly into the residual. The arithmetic and selected backend stay the same,
but the obsolete constructor allocation remains resident for the lifetime of
the layer.

PS6105 deliberately proves only a narrow, source-visible candidate. The field
must be unexported on a package-local, non-generic struct and initialized by one
reachable straight-line constructor with a fixed positive make([]T, len[, cap]).
The element layout and backing capacity must be known without type-parameter or
integer-overflow uncertainty. Every typed use of that exact field object must
be either its supported constructor initialization or the complete argument to
a package-local concrete function or method whose corresponding formal
parameter has zero uses anywhere in its body. A field with no reads also
qualifies.

Interface dispatch, imported or indirect calls, generic and variadic targets,
alternate field initializers, unkeyed literals, reassignment, address-taking,
capture, indexing, slicing, len/cap, append, nil comparison, return, storage,
conversion, and every unclassified use suppress the finding. A concrete call
reached after an explicit type assertion can qualify; a backend-name or guard
heuristic cannot. Preserve interface and backend capability gates when
performing the cleanup.

There is NO automatic fix. Removing only the initializer changes the field's
nil state, length, capacity, and identity. Removing the field also changes
layout. Reflection, formatting, unsafe code, whole-receiver escape, alternate
build-tag implementations, and external callers can observe facts that a typed
field-use scan cannot close. Audit those boundaries, remove the obsolete field,
argument, and formal together only when appropriate, and benchmark the real
retained lifetime. A source-proven allocation reduction is not by itself a
wall-time win.`,
		Before: `type projection struct {
	scratch []float32
}

func newProjection() *projection {
	return &projection{scratch: make([]float32, 1024*768)}
}

func (p *projection) Forward(linear *linearF32, residual []float32) {
	linear.Accumulate(residual, p.scratch)
}

func (l *linearF32) Accumulate(residual, scratch []float32) {
	// Accumulating epilogue writes directly to residual; scratch is unused.
}`,
		After: `type projection struct{}

func newProjection() *projection { return &projection{} }

func (p *projection) Forward(linear *linearF32, residual []float32) {
	linear.Accumulate(residual)
}

func (l *linearF32) Accumulate(residual []float32) {
	// Preserve the same accumulating backend path.
}`,
		MeasuredWin: `Owner issue #884 removed two retained 1024x768 float32
scratch buffers after an accumulating matmul epilogue made them obsolete. The
resident payload fell by 6,291,456 bytes without changing arithmetic or
dispatch. Treat that as allocation evidence only: validate the exact backend,
outputs, retained lifetime, and wall-time crossover in the affected project.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6105",
		Doc:  "constructor-stored projection scratch has no ordinary typed consumer",
		Run:  runPS6105,
	},
})

type ps6105Candidate struct {
	field      *types.Var
	owner      *types.Named
	allocation *ast.CallExpr
	fieldUse   *ast.Ident
	bytes      int64
}

func runPS6105(pass *analysis.Pass) (any, error) {
	fields := ps6105Fields(pass)
	locals := ps6105LocalFunctions(pass)
	candidates := ps6105Candidates(pass, fields)
	byField := make(map[*types.Var][]ps6105Candidate, len(candidates))
	for _, candidate := range candidates {
		byField[candidate.field] = append(byField[candidate.field], candidate)
	}
	for field, matches := range byField {
		// Multiple supported writes still leave instance and overwrite ordering
		// unresolved. The first bounded form is one allocation site per field.
		if len(matches) != 1 {
			continue
		}
		candidate := matches[0]
		if ps6105HasUnkeyedLiteral(pass, candidate.owner) {
			continue
		}
		calls, ok := ps6105FieldUses(pass, field, candidate.fieldUse, locals)
		if !ok {
			continue
		}
		useProof := "has no source-visible field read"
		if calls > 0 {
			useProof = "is read only as a whole argument to zero-use formals of closed package-local concrete callees"
		}
		pass.Report(analysis.Diagnostic{
			Pos: candidate.allocation.Pos(),
			End: candidate.allocation.End(),
			Message: "constructor stores a fixed " + strconv.FormatInt(candidate.bytes, 10) + "-byte " +
				candidate.owner.Obj().Name() + "." + field.Name() + " scratch-allocation candidate that " + useProof +
				"; evaluate whether it is obsolete retained projection scratch while preserving interface/backend gates, then audit reflection, formatting, unsafe/layout, whole-receiver escape and identity, alternate build tags, exact outputs, retained memory, and wall time (source-visible advisory, no automatic fix)",
		})
	}
	return nil, nil
}

func ps6105Fields(pass *analysis.Pass) map[*types.Var]*types.Named {
	result := make(map[*types.Var]*types.Named)
	scope := pass.Pkg.Scope()
	for _, name := range scope.Names() {
		typeName, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := types.Unalias(typeName.Type()).(*types.Named)
		if !ok || named.TypeParams().Len() != 0 {
			continue
		}
		structure, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		for index := range structure.NumFields() {
			field := structure.Field(index)
			if field.Pkg() == pass.Pkg && !field.Exported() && field.Name() != "_" {
				result[field] = named
			}
		}
	}
	return result
}

func ps6105LocalFunctions(pass *analysis.Pass) map[*types.Func]*ast.FuncDecl {
	result := make(map[*types.Func]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if ok {
				result[object.Origin()] = function
			}
		}
	}
	return result
}

func ps6105Candidates(pass *analysis.Pass, fields map[*types.Var]*types.Named) []ps6105Candidate {
	var result []ps6105Candidate
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			owner, returned, ok := ps6105Constructor(pass, function)
			if !ok {
				continue
			}
			parents := ps6087Parents(function.Body)
			reachable := ps6099ReachableNodesInBlock(pass, function.Body, parents)
			if !reachable[returned] {
				continue
			}
			literal, root, ok := ps6105ConstructorValue(pass, function, returned, owner, parents, reachable)
			if !ok {
				continue
			}
			result = append(result, ps6105LiteralCandidates(pass, literal, owner, fields)...)
			if root != nil {
				result = append(result, ps6105AssignmentCandidates(pass, function.Body, root, owner, fields, reachable)...)
			}
		}
	}
	return result
}

func ps6105Constructor(pass *analysis.Pass, function *ast.FuncDecl) (*types.Named, *ast.ReturnStmt, bool) {
	object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
	if !ok {
		return nil, nil, false
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok || signature.Recv() != nil || signature.TypeParams().Len() != 0 || signature.Results().Len() != 1 || len(function.Body.List) == 0 {
		return nil, nil, false
	}
	pointer, ok := types.Unalias(signature.Results().At(0).Type()).(*types.Pointer)
	if !ok {
		return nil, nil, false
	}
	owner, ok := types.Unalias(pointer.Elem()).(*types.Named)
	if !ok || owner.Obj().Pkg() != pass.Pkg || owner.TypeParams().Len() != 0 {
		return nil, nil, false
	}
	if _, ok := owner.Underlying().(*types.Struct); !ok {
		return nil, nil, false
	}
	returned, ok := function.Body.List[len(function.Body.List)-1].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return nil, nil, false
	}
	count := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if _, ok := node.(*ast.ReturnStmt); ok {
			count++
		}
		return true
	})
	return owner, returned, count == 1
}

func ps6105ConstructorValue(pass *analysis.Pass, function *ast.FuncDecl, returned *ast.ReturnStmt, owner *types.Named, parents map[ast.Node]ast.Node, reachable map[ast.Node]bool) (*ast.CompositeLit, types.Object, bool) {
	if literal := ps6105AddressedLiteral(pass, returned.Results[0], owner); literal != nil {
		return literal, nil, true
	}
	identifier, ok := ps2110Unparen(returned.Results[0]).(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	root := pass.TypesInfo.Uses[identifier]
	if _, ok := root.(*types.Var); !ok || !ps6105RootUsesSafe(pass, function.Body, returned, root, parents) {
		return nil, nil, false
	}
	var literal *ast.CompositeLit
	for _, statement := range function.Body.List[:len(function.Body.List)-1] {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		left, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
		if !ok || pass.TypesInfo.Defs[left] != root {
			continue
		}
		candidate := ps6105AddressedLiteral(pass, assignment.Rhs[0], owner)
		if candidate == nil || !reachable[assignment] || literal != nil {
			return nil, nil, false
		}
		literal = candidate
	}
	return literal, root, literal != nil
}

func ps6105AddressedLiteral(pass *analysis.Pass, expression ast.Expr, owner *types.Named) *ast.CompositeLit {
	unary, ok := ps2110Unparen(expression).(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return nil
	}
	literal, ok := ps2110Unparen(unary.X).(*ast.CompositeLit)
	if !ok {
		return nil
	}
	typeOf, ok := types.Unalias(pass.TypesInfo.TypeOf(literal)).(*types.Named)
	if !ok || typeOf != owner {
		return nil
	}
	return literal
}

func ps6105RootUsesSafe(pass *analysis.Pass, body *ast.BlockStmt, returned *ast.ReturnStmt, root types.Object, parents map[ast.Node]ast.Node) bool {
	safe := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !safe {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != root {
			return true
		}
		if pass.TypesInfo.Defs[identifier] == root {
			return true
		}
		for parent := parents[identifier]; parent != nil && parent != body; parent = parents[parent] {
			if _, captured := parent.(*ast.FuncLit); captured {
				safe = false
				return false
			}
		}
		expression := ast.Expr(identifier)
		for {
			parenthesis, ok := parents[expression].(*ast.ParenExpr)
			if !ok || parenthesis.X != expression {
				break
			}
			expression = parenthesis
		}
		if parents[expression] == returned && ps2110Unparen(returned.Results[0]) == identifier {
			return true
		}
		selector, ok := parents[expression].(*ast.SelectorExpr)
		if !ok || selector.X != expression {
			safe = false
			return false
		}
		selection := pass.TypesInfo.Selections[selector]
		if selection == nil || selection.Kind() != types.FieldVal {
			safe = false
			return false
		}
		return true
	})
	return safe
}

func ps6105LiteralCandidates(pass *analysis.Pass, literal *ast.CompositeLit, owner *types.Named, fields map[*types.Var]*types.Named) []ps6105Candidate {
	var result []ps6105Candidate
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ps2110Unparen(pair.Key).(*ast.Ident)
		if !ok {
			continue
		}
		field, ok := pass.TypesInfo.ObjectOf(key).(*types.Var)
		if !ok || fields[field] != owner {
			continue
		}
		allocation, bytes, ok := ps6105FixedMake(pass, pair.Value, field.Type())
		if ok {
			result = append(result, ps6105Candidate{field: field, owner: owner, allocation: allocation, fieldUse: key, bytes: bytes})
		}
	}
	return result
}

func ps6105AssignmentCandidates(pass *analysis.Pass, body *ast.BlockStmt, root types.Object, owner *types.Named, fields map[*types.Var]*types.Named, reachable map[ast.Node]bool) []ps6105Candidate {
	var result []ps6105Candidate
	for _, statement := range body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !reachable[assignment] {
			continue
		}
		selector, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.SelectorExpr)
		if !ok || !ps6105Object(pass, selector.X, root) {
			continue
		}
		selection := pass.TypesInfo.Selections[selector]
		field, ok := selectionObject(selection).(*types.Var)
		if !ok || selection.Kind() != types.FieldVal || fields[field] != owner {
			continue
		}
		allocation, bytes, ok := ps6105FixedMake(pass, assignment.Rhs[0], field.Type())
		if ok {
			result = append(result, ps6105Candidate{field: field, owner: owner, allocation: allocation, fieldUse: selector.Sel, bytes: bytes})
		}
	}
	return result
}

func ps6105Object(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(identifier) == object
}

func ps6105FixedMake(pass *analysis.Pass, expression ast.Expr, fieldType types.Type) (*ast.CallExpr, int64, bool) {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || !typedBuiltinName(pass, call.Fun, "make") || len(call.Args) < 2 || len(call.Args) > 3 || call.Ellipsis.IsValid() {
		return nil, 0, false
	}
	callType := pass.TypesInfo.TypeOf(call)
	if callType == nil {
		return nil, 0, false
	}
	slice, ok := types.Unalias(callType).Underlying().(*types.Slice)
	if !ok || !types.Identical(callType, fieldType) || !ps6105KnownNonzeroSize(pass.TypesSizes, slice.Elem()) {
		return nil, 0, false
	}
	length, ok := ps6105PositiveInt(pass, call.Args[1])
	if !ok {
		return nil, 0, false
	}
	capacity := length
	if len(call.Args) == 3 {
		capacity, ok = ps6105PositiveInt(pass, call.Args[2])
		if !ok || capacity < length {
			return nil, 0, false
		}
	}
	intBytes := pass.TypesSizes.Sizeof(types.Typ[types.Int])
	if intBytes <= 0 || intBytes > 8 {
		return nil, 0, false
	}
	maxInt := int64(math.MaxInt64)
	if intBytes < 8 {
		maxInt = int64(1)<<(uint(intBytes)*8-1) - 1
	}
	elementBytes := pass.TypesSizes.Sizeof(slice.Elem())
	if capacity > maxInt || elementBytes <= 0 || capacity > math.MaxInt64/elementBytes {
		return nil, 0, false
	}
	return call, capacity * elementBytes, true
}

func ps6105KnownNonzeroSize(sizes types.Sizes, value types.Type) (ok bool) {
	if sizes == nil || ps6105ContainsTypeParam(value, make(map[types.Type]bool)) {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return sizes.Sizeof(value) > 0
}

func ps6105ContainsTypeParam(value types.Type, seen map[types.Type]bool) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	switch value := value.(type) {
	case *types.TypeParam:
		return true
	case *types.Array:
		return ps6105ContainsTypeParam(value.Elem(), seen)
	case *types.Slice:
		return ps6105ContainsTypeParam(value.Elem(), seen)
	case *types.Pointer:
		return ps6105ContainsTypeParam(value.Elem(), seen)
	case *types.Map:
		return ps6105ContainsTypeParam(value.Key(), seen) || ps6105ContainsTypeParam(value.Elem(), seen)
	case *types.Chan:
		return ps6105ContainsTypeParam(value.Elem(), seen)
	case *types.Struct:
		for index := range value.NumFields() {
			if ps6105ContainsTypeParam(value.Field(index).Type(), seen) {
				return true
			}
		}
	case *types.Tuple:
		for index := range value.Len() {
			if ps6105ContainsTypeParam(value.At(index).Type(), seen) {
				return true
			}
		}
	case *types.Signature:
		for _, parameters := range []*types.TypeParamList{value.RecvTypeParams(), value.TypeParams()} {
			for index := 0; parameters != nil && index < parameters.Len(); index++ {
				if ps6105ContainsTypeParam(parameters.At(index), seen) {
					return true
				}
			}
		}
		return ps6105ContainsTypeParam(value.Params(), seen) || ps6105ContainsTypeParam(value.Results(), seen)
	case *types.Named:
		for index := 0; value.TypeArgs() != nil && index < value.TypeArgs().Len(); index++ {
			if ps6105ContainsTypeParam(value.TypeArgs().At(index), seen) {
				return true
			}
		}
		return ps6105ContainsTypeParam(value.Underlying(), seen)
	case *types.Interface:
		for index := range value.NumEmbeddeds() {
			if ps6105ContainsTypeParam(value.EmbeddedType(index), seen) {
				return true
			}
		}
		for index := range value.NumExplicitMethods() {
			if ps6105ContainsTypeParam(value.ExplicitMethod(index).Type(), seen) {
				return true
			}
		}
	case *types.Union:
		for index := range value.Len() {
			if ps6105ContainsTypeParam(value.Term(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func ps6105PositiveInt(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	integer, exact := constant.Int64Val(value)
	return integer, exact && integer > 0
}

func ps6105HasUnkeyedLiteral(pass *analysis.Pass, owner *types.Named) bool {
	for _, file := range pass.Files {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			if found {
				return false
			}
			literal, ok := node.(*ast.CompositeLit)
			if !ok || len(literal.Elts) == 0 {
				return true
			}
			typeOf, ok := types.Unalias(pass.TypesInfo.TypeOf(literal)).(*types.Named)
			if !ok || typeOf != owner {
				return true
			}
			if _, keyed := literal.Elts[0].(*ast.KeyValueExpr); !keyed {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func ps6105FieldUses(pass *analysis.Pass, field *types.Var, initializer *ast.Ident, locals map[*types.Func]*ast.FuncDecl) (int, bool) {
	calls := 0
	for _, file := range pass.Files {
		parents := ps6087Parents(file)
		safe := true
		ast.Inspect(file, func(node ast.Node) bool {
			if !safe {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if !ok || pass.TypesInfo.ObjectOf(identifier) != field || pass.TypesInfo.Defs[identifier] == field {
				return true
			}
			if identifier == initializer {
				return true
			}
			selector, ok := parents[identifier].(*ast.SelectorExpr)
			if !ok || selector.Sel != identifier {
				safe = false
				return false
			}
			selection := pass.TypesInfo.Selections[selector]
			if selection == nil || selection.Kind() != types.FieldVal || selection.Obj() != field {
				safe = false
				return false
			}
			for parent := parents[selector]; parent != nil; parent = parents[parent] {
				if _, captured := parent.(*ast.FuncLit); captured {
					safe = false
					return false
				}
			}
			call, argument, ok := ps6105WholeArgument(selector, parents)
			if !ok || !ps6105UnusedLocalFormal(pass, call, argument, locals) {
				safe = false
				return false
			}
			calls++
			return true
		})
		if !safe {
			return 0, false
		}
	}
	return calls, true
}

func ps6105WholeArgument(expression ast.Expr, parents map[ast.Node]ast.Node) (*ast.CallExpr, int, bool) {
	var node ast.Node = expression
	for {
		parenthesis, ok := parents[node].(*ast.ParenExpr)
		if !ok || parenthesis.X != node {
			break
		}
		node = parenthesis
	}
	call, ok := parents[node].(*ast.CallExpr)
	if !ok {
		return nil, 0, false
	}
	for index, argument := range call.Args {
		if argument == node {
			return call, index, true
		}
	}
	return nil, 0, false
}

func ps6105UnusedLocalFormal(pass *analysis.Pass, call *ast.CallExpr, argument int, locals map[*types.Func]*ast.FuncDecl) bool {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok || function.Pkg() != pass.Pkg || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 ||
		!ps6099ConcreteLeafCallee(pass, call, function, signature) {
		return false
	}
	offset, ok := ps6099CallSignatureOffset(pass, call, signature)
	parameterIndex := argument - offset
	if !ok || parameterIndex < 0 || parameterIndex >= signature.Params().Len() {
		return false
	}
	declaration := locals[function.Origin()]
	if declaration == nil || declaration.Body == nil {
		return false
	}
	parameter := signature.Params().At(parameterIndex)
	unused := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[identifier] == parameter {
			unused = false
			return false
		}
		return unused
	})
	return unused
}
