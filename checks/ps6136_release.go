package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// ps6136ReleaseList proves the source loop releases every non-nil retained
// element once and immediately drops the list. This is not a universal
// lifecycle proof: the caller must close all list observations, aliases and
// other release paths, and bind the reviewed native release implementation.
func ps6136ReleaseList(pass *analysis.Pass, declaration *ast.FuncDecl, receiver types.Object, list *types.Var, release *types.Func) map[ast.Node]bool {
	if pass == nil || declaration == nil || declaration.Body == nil || receiver == nil || list == nil || release == nil {
		return nil
	}
	var approved map[ast.Node]bool
	for index, statement := range declaration.Body.List {
		loop, ok := statement.(*ast.RangeStmt)
		if !ok || loop.Tok != token.DEFINE || index+1 >= len(declaration.Body.List) || !ps6136DirectField(pass, loop.X, receiver, list) {
			continue
		}
		if approved != nil || len(loop.Body.List) != 1 {
			return nil
		}
		// The cleanup must be reached on every normally returning invocation.
		// Permit straight-line expression prefixes (the authentic shared owner
		// drains pending work here), but no conditional return, label/goto,
		// nested control flow, deferred or asynchronous prefix execution.
		for _, prefix := range declaration.Body.List[:index] {
			if _, ok := prefix.(*ast.ExprStmt); !ok {
				return nil
			}
		}
		value, ok := loop.Value.(*ast.Ident)
		if !ok || pass.TypesInfo.Defs[value] == nil {
			return nil
		}
		guard, ok := loop.Body.List[0].(*ast.IfStmt)
		if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
			return nil
		}
		condition, ok := guard.Cond.(*ast.BinaryExpr)
		if !ok || condition.Op != token.NEQ || !ps6136ObjectExpr(pass, condition.X, pass.TypesInfo.Defs[value]) || !ps6136NilExpr(pass, condition.Y) {
			return nil
		}
		expression, ok := guard.Body.List[0].(*ast.ExprStmt)
		if !ok {
			return nil
		}
		call, ok := expression.X.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 || call.Ellipsis.IsValid() {
			return nil
		}
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !ps6136ObjectExpr(pass, method.X, pass.TypesInfo.Defs[value]) {
			return nil
		}
		selection := pass.TypesInfo.Selections[method]
		if selection == nil || selection.Obj() != release {
			return nil
		}
		signature, ok := release.Type().(*types.Signature)
		if !ok || signature.Variadic() || signature.Params().Len() != 0 || signature.Results().Len() != 0 {
			return nil
		}
		clear, ok := declaration.Body.List[index+1].(*ast.AssignStmt)
		if !ok || clear.Tok != token.ASSIGN || len(clear.Lhs) != 1 || len(clear.Rhs) != 1 || !ps6136DirectField(pass, clear.Lhs[0], receiver, list) || !ps6136NilExpr(pass, clear.Rhs[0]) {
			return nil
		}
		approved = map[ast.Node]bool{loop.X: true, clear.Lhs[0]: true}
	}
	return approved
}

func ps6136ObjectExpr(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && object != nil && pass.TypesInfo.Uses[identifier] == object
}

func ps6136NilExpr(pass *analysis.Pass, expression ast.Expr) bool {
	return ps6136ObjectExpr(pass, expression, types.Universe.Lookup("nil"))
}

func ps6136DirectField(pass *analysis.Pass, expression ast.Expr, receiver types.Object, field *types.Var) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || !ps6136ObjectExpr(pass, selector.X, receiver) {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	return selection != nil && selection.Kind() == types.FieldVal && selection.Obj() == field
}
