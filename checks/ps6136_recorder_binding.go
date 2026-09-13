package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

func (selection *ps6136Selection) recorderBinding(pass *analysis.Pass, recorderField *types.Var, nativeID string, wrapper *types.Named, field *types.Var) bool {
	if selection == nil || recorderField == nil || selection.ops == nil {
		return false
	}
	snapshot := ps6136InitialOwnerField(selection.owner, selection.ops)
	if snapshot.value == nil {
		return false
	}
	structure, ok := selection.ops.Type().Underlying().(*types.Struct)
	if !ok {
		return false
	}
	index := -1
	for candidate := 0; candidate < structure.NumFields(); candidate++ {
		if structure.Field(candidate) == recorderField {
			index = candidate
		}
	}
	if index < 0 {
		return false
	}
	origin := snapshot.context.structField(snapshot.value, index)
	if origin.value == nil || origin.context == nil {
		return false
	}
	function, captures := ps6125SSACallee(origin.value)
	if function == nil || len(captures) != 0 || len(function.FreeVars) != 0 {
		return false
	}
	literal, ok := function.Syntax().(*ast.FuncLit)
	return ok && ps6136RecorderLiteral(pass, literal, nativeID, wrapper, field)
}

// Bind the selected backend's source literal to its exact native recorder.
// This closes the factory's argument/result flow, not the native API's access,
// scheduling, synchronization or lifetime semantics.
func ps6136RecorderLiteral(pass *analysis.Pass, literal *ast.FuncLit, nativeID string, wrapper *types.Named, field *types.Var) bool {
	if wrapper == nil || field == nil {
		return false
	}
	return ps6136RecorderFactory(pass, literal, nativeID, field.Type(), nil, func(expression ast.Expr, result types.Object) bool {
		value, ok := expression.(*ast.CompositeLit)
		if !ok || !types.Identical(pass.TypesInfo.TypeOf(value), wrapper) || len(value.Elts) != 1 {
			return false
		}
		structure, ok := wrapper.Underlying().(*types.Struct)
		if !ok || structure.NumFields() != 1 || structure.Field(0) != field {
			return false
		}
		identifier, ok := value.Elts[0].(*ast.Ident)
		return ok && pass.TypesInfo.Uses[identifier] == result
	})
}

func ps6136RecorderFactory(pass *analysis.Pass, literal *ast.FuncLit, nativeID string, nativeType types.Type, arguments []types.Object, resultProof func(ast.Expr, types.Object) bool) bool {
	if pass == nil || literal == nil || nativeType == nil || resultProof == nil || len(literal.Body.List) != 3 {
		return false
	}
	signature, ok := pass.TypesInfo.TypeOf(literal).(*types.Signature)
	if !ok || signature.Variadic() || signature.Params().Len() != 0 || signature.Results().Len() != 2 || !types.Identical(signature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return false
	}
	assignment, ok := literal.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
		return false
	}
	result, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Defs[result] == nil {
		return false
	}
	errorName, ok := assignment.Lhs[1].(*ast.Ident)
	if !ok || pass.TypesInfo.Defs[errorName] == nil {
		return false
	}
	call, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != len(arguments) || call.Ellipsis.IsValid() {
		return false
	}
	callee, nativeSignature, ok := typedCallee(pass, call.Fun)
	if !ok || ps6090FunctionID(callee) != nativeID || nativeSignature.Recv() != nil || nativeSignature.Variadic() || nativeSignature.TypeParams().Len() != 0 || nativeSignature.Params().Len() != len(arguments) || nativeSignature.Results().Len() != 2 || !types.Identical(nativeSignature.Results().At(0).Type(), nativeType) || !types.Identical(nativeSignature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return false
	}
	for index, object := range arguments {
		identifier, ok := call.Args[index].(*ast.Ident)
		if !ok || object == nil || pass.TypesInfo.Uses[identifier] != object || !types.Identical(object.Type(), nativeSignature.Params().At(index).Type()) {
			return false
		}
	}
	guard, ok := literal.Body.List[1].(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	condition, ok := guard.Cond.(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ {
		return false
	}
	identical := func(expression ast.Expr, object types.Object) bool {
		identifier, ok := expression.(*ast.Ident)
		return ok && pass.TypesInfo.Uses[identifier] == object
	}
	nilValue := func(expression ast.Expr) bool { return identical(expression, types.Universe.Lookup("nil")) }
	if !identical(condition.X, pass.TypesInfo.Defs[errorName]) || !nilValue(condition.Y) {
		return false
	}
	failure, ok := guard.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(failure.Results) != 2 || !nilValue(failure.Results[0]) || !identical(failure.Results[1], pass.TypesInfo.Defs[errorName]) {
		return false
	}
	publication, ok := literal.Body.List[2].(*ast.ReturnStmt)
	if !ok || len(publication.Results) != 2 || !nilValue(publication.Results[1]) {
		return false
	}
	return resultProof(publication.Results[0], pass.TypesInfo.Defs[result])
}
