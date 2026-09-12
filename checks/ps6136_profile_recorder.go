package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// The profiler may add metadata, never a second native recorder choice. The
// operation method set must still select the exact base-adapter methods.
func ps6136ProfileRecorderFactory(pass *analysis.Pass, literal *ast.FuncLit, nativeID string, capacity, metadata types.Object, wrapper, base *types.Named, embedded, profile, nativeField *types.Var) bool {
	if wrapper == nil || base == nil || embedded == nil || profile == nil || nativeField == nil || metadata == nil || capacity == nil {
		return false
	}
	structure, ok := wrapper.Underlying().(*types.Struct)
	if !ok || structure.NumFields() != 2 || structure.Field(0) != embedded || structure.Field(1) != profile || !embedded.Embedded() || !types.Identical(embedded.Type(), base) || !types.Identical(profile.Type(), types.NewPointer(metadata.Type())) {
		return false
	}
	baseStructure, ok := base.Underlying().(*types.Struct)
	if !ok || baseStructure.NumFields() != 1 || baseStructure.Field(0) != nativeField || !types.Identical(capacity.Type(), types.Typ[types.Int]) {
		return false
	}
	return ps6136RecorderFactory(pass, literal, nativeID, nativeField.Type(), []types.Object{capacity}, func(expression ast.Expr, result types.Object) bool {
		value, ok := expression.(*ast.CompositeLit)
		if !ok || !types.Identical(pass.TypesInfo.TypeOf(value), wrapper) || len(value.Elts) != 2 {
			return false
		}
		var nativeValue, metadataValue ast.Expr
		for _, element := range value.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				return false
			}
			key, ok := pair.Key.(*ast.Ident)
			if !ok {
				return false
			}
			switch pass.TypesInfo.Uses[key] {
			case embedded:
				if nativeValue != nil {
					return false
				}
				nativeValue = pair.Value
			case profile:
				if metadataValue != nil {
					return false
				}
				metadataValue = pair.Value
			default:
				return false
			}
		}
		baseValue, ok := nativeValue.(*ast.CompositeLit)
		if !ok || !types.Identical(pass.TypesInfo.TypeOf(baseValue), base) || len(baseValue.Elts) != 1 {
			return false
		}
		pair, ok := baseValue.Elts[0].(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[key] != nativeField {
			return false
		}
		identifier, ok := pair.Value.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != result {
			return false
		}
		address, ok := metadataValue.(*ast.UnaryExpr)
		if !ok || address.Op != token.AND {
			return false
		}
		identifier, ok = address.X.(*ast.Ident)
		return ok && pass.TypesInfo.Uses[identifier] == metadata
	})
}

func ps6136SameAdapterMethods(profile, base *types.Named, methods []*types.Func) bool {
	if profile == nil || base == nil || len(methods) == 0 {
		return false
	}
	for _, method := range methods {
		if method == nil {
			return false
		}
		object, _, _ := types.LookupFieldOrMethod(profile, false, method.Pkg(), method.Name())
		if object != method {
			return false
		}
		object, _, _ = types.LookupFieldOrMethod(base, false, method.Pkg(), method.Name())
		if object != method {
			return false
		}
	}
	return true
}

// Exact source override/restore frame. It returns only the concrete assignment
// targets whose state changes are justified, never a function-wide exemption.
func ps6136ProfileOverrideFrame(pass *analysis.Pass, declaration *ast.FuncDecl, ops, primary, secondary, async *types.Var, stepID, dropID string, factoryProof func(*ast.FuncLit, types.Object, types.Object) bool) map[ast.Node]bool {
	if pass == nil || declaration == nil || declaration.Body == nil || declaration.Recv == nil || len(declaration.Recv.List) != 1 || len(declaration.Recv.List[0].Names) != 1 || len(declaration.Body.List) != 14 || factoryProof == nil {
		return nil
	}
	function, ok := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if !ok {
		return nil
	}
	signature := function.Type().(*types.Signature)
	if signature.Params().Len() != 3 || signature.Results().Len() != 3 {
		return nil
	}
	receiver := pass.TypesInfo.Defs[declaration.Recv.List[0].Names[0]]
	member := func(expression ast.Expr, field *types.Var) bool {
		selector, ok := expression.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		selection := pass.TypesInfo.Selections[selector]
		base, ok := selector.X.(*ast.SelectorExpr)
		if !ok || selection == nil || selection.Kind() != types.FieldVal || selection.Obj() != field {
			return false
		}
		selection = pass.TypesInfo.Selections[base]
		identifier, ok := base.X.(*ast.Ident)
		return ok && pass.TypesInfo.Uses[identifier] == receiver && selection != nil && selection.Kind() == types.FieldVal && selection.Obj() == ops
	}
	identical := func(expression ast.Expr, object types.Object) bool {
		identifier, ok := expression.(*ast.Ident)
		return ok && pass.TypesInfo.Uses[identifier] == object
	}
	for _, statement := range declaration.Body.List[:4] {
		guard, ok := statement.(*ast.IfStmt)
		if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
			return nil
		}
		returned, ok := guard.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(returned.Results) != 3 || !identical(returned.Results[0], types.Universe.Lookup("nil")) {
			return nil
		}
	}
	callStatement, ok := declaration.Body.List[4].(*ast.ExprStmt)
	if !ok {
		return nil
	}
	drop, ok := callStatement.X.(*ast.CallExpr)
	if !ok || len(drop.Args) != 0 {
		return nil
	}
	callee, _, typed := typedCallee(pass, drop.Fun)
	selector, selected := drop.Fun.(*ast.SelectorExpr)
	if !typed || ps6090FunctionID(callee) != dropID || !selected || !identical(selector.X, receiver) {
		return nil
	}
	saved, ok := declaration.Body.List[5].(*ast.AssignStmt)
	if !ok || saved.Tok != token.DEFINE || len(saved.Lhs) != 3 || len(saved.Rhs) != 3 {
		return nil
	}
	fields := []*types.Var{primary, secondary, async}
	previous := make([]types.Object, 3)
	for index, field := range fields {
		identifier, ok := saved.Lhs[index].(*ast.Ident)
		if !ok || pass.TypesInfo.Defs[identifier] == nil || !member(saved.Rhs[index], field) {
			return nil
		}
		previous[index] = pass.TypesInfo.Defs[identifier]
	}
	deferred, ok := declaration.Body.List[6].(*ast.DeferStmt)
	if !ok || len(deferred.Call.Args) != 0 {
		return nil
	}
	restore, ok := deferred.Call.Fun.(*ast.FuncLit)
	if !ok || len(restore.Body.List) != 3 {
		return nil
	}
	proved := make(map[ast.Node]bool, len(restore.Body.List)+3)
	for index, statement := range restore.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !member(assignment.Lhs[0], fields[index]) || !identical(assignment.Rhs[0], previous[index]) {
			return nil
		}
		proved[assignment.Lhs[0]] = true
	}
	metadataDeclaration, ok := declaration.Body.List[7].(*ast.DeclStmt)
	if !ok {
		return nil
	}
	general, ok := metadataDeclaration.Decl.(*ast.GenDecl)
	if !ok || general.Tok != token.VAR || len(general.Specs) != 1 {
		return nil
	}
	metadataValue, ok := general.Specs[0].(*ast.ValueSpec)
	if !ok || len(metadataValue.Names) != 1 || len(metadataValue.Values) != 0 {
		return nil
	}
	metadata := pass.TypesInfo.Defs[metadataValue.Names[0]]
	factoryAssignment, ok := declaration.Body.List[8].(*ast.AssignStmt)
	if !ok || factoryAssignment.Tok != token.ASSIGN || len(factoryAssignment.Lhs) != 1 || len(factoryAssignment.Rhs) != 1 || !member(factoryAssignment.Lhs[0], primary) {
		return nil
	}
	literal, ok := factoryAssignment.Rhs[0].(*ast.FuncLit)
	if !ok || !factoryProof(literal, signature.Params().At(2), metadata) {
		return nil
	}
	proved[factoryAssignment.Lhs[0]] = true
	alias, ok := declaration.Body.List[9].(*ast.AssignStmt)
	if !ok || alias.Tok != token.ASSIGN || len(alias.Lhs) != 1 || len(alias.Rhs) != 1 || !member(alias.Lhs[0], secondary) || !member(alias.Rhs[0], primary) {
		return nil
	}
	proved[alias.Lhs[0]] = true
	synchronous, ok := declaration.Body.List[10].(*ast.AssignStmt)
	if !ok || synchronous.Tok != token.ASSIGN || len(synchronous.Lhs) != 1 || len(synchronous.Rhs) != 1 || !member(synchronous.Lhs[0], async) || !identical(synchronous.Rhs[0], types.Universe.Lookup("false")) {
		return nil
	}
	proved[synchronous.Lhs[0]] = true
	step, ok := declaration.Body.List[11].(*ast.AssignStmt)
	if !ok || step.Tok != token.DEFINE || len(step.Lhs) != 2 || len(step.Rhs) != 1 {
		return nil
	}
	call, ok := step.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || !identical(call.Args[0], signature.Params().At(0)) || !identical(call.Args[1], signature.Params().At(1)) {
		return nil
	}
	callee, _, typed = typedCallee(pass, call.Fun)
	selector, selected = call.Fun.(*ast.SelectorExpr)
	if !typed || ps6090FunctionID(callee) != stepID || !selected || !identical(selector.X, receiver) {
		return nil
	}
	// No late operations may write the saved callback cells or bypass restoration.
	guard, ok := declaration.Body.List[12].(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return nil
	}
	logits, logitsOK := step.Lhs[0].(*ast.Ident)
	errorName, errorOK := step.Lhs[1].(*ast.Ident)
	if !logitsOK || !errorOK || pass.TypesInfo.Defs[logits] == nil || pass.TypesInfo.Defs[errorName] == nil {
		return nil
	}
	comparison, ok := guard.Cond.(*ast.BinaryExpr)
	if !ok || comparison.Op != token.NEQ || !identical(comparison.X, pass.TypesInfo.Defs[errorName]) || !identical(comparison.Y, types.Universe.Lookup("nil")) {
		return nil
	}
	failure, ok := guard.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(failure.Results) != 3 || !identical(failure.Results[0], types.Universe.Lookup("nil")) || !identical(failure.Results[2], pass.TypesInfo.Defs[errorName]) {
		return nil
	}
	emptyProfile, ok := failure.Results[1].(*ast.CompositeLit)
	if !ok || len(emptyProfile.Elts) != 0 || !types.Identical(pass.TypesInfo.TypeOf(emptyProfile), metadata.Type()) {
		return nil
	}
	publication, ok := declaration.Body.List[13].(*ast.ReturnStmt)
	if !ok || len(publication.Results) != 3 || !identical(publication.Results[0], pass.TypesInfo.Defs[logits]) || !identical(publication.Results[1], metadata) || !identical(publication.Results[2], types.Universe.Lookup("nil")) {
		return nil
	}
	return proved
}
