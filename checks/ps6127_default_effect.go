package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// All expression positions can call a helper, and a field address or wrapper
// can expose the same constructed config. Only a direct, simple actual admits
// the exact clear/observation summary; every other exposure stays Unknown.
func ps6127DefaultStatementEffects(pass *analysis.Pass, functions map[string]*ast.FuncDecl, statement ast.Stmt, regexField *types.Var, built, unknown, constructed map[types.Object]bool) {
	ast.Inspect(statement, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
			ps6127UnknownExposures(pass, selector.X, built, unknown, constructed)
		}
		simpleCall := true
		for _, argument := range call.Args {
			ast.Inspect(argument, func(n ast.Node) bool {
				if _, ok := n.(*ast.CallExpr); ok {
					simpleCall = false
					return false
				}
				return true
			})
		}
		for index, argument := range call.Args {
			object := ps6114ExprObject(pass, argument)
			direct, isIdent := ps2110Unparen(argument).(*ast.Ident)
			expression, topLevel := statement.(*ast.ExprStmt)
			if simpleCall && isIdent && direct != nil && constructed[object] && topLevel && ps2110Unparen(expression.X) == call {
				switch ps6127DefaultCallEffect(pass, functions, call, index, regexField) {
				case ps6127PolicyAbsent:
					built[object], unknown[object] = false, false
				case ps6127PolicyUnknown:
					built[object], unknown[object] = false, true
				}
			} else {
				ps6127UnknownExposures(pass, argument, built, unknown, constructed)
			}
		}
		return true
	})
}

func ps6127UnknownExposures(pass *analysis.Pass, expression ast.Expr, built, unknown, constructed map[types.Object]bool) {
	ast.Inspect(expression, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			if object := pass.TypesInfo.ObjectOf(identifier); constructed[object] {
				built[object], unknown[object] = false, true
			}
		}
		return true
	})
}

// ps6127DefaultCallEffect summarizes the deliberately small helper grammar
// used after construction of the default selector configuration. Absent means
// the exact consumed regexp field is unconditionally cleared; Covered means
// the helper only observes that field; Unknown means no preservation or clear
// can be proved.
func ps6127DefaultCallEffect(pass *analysis.Pass, functions map[string]*ast.FuncDecl, call *ast.CallExpr, argumentIndex int, regexField *types.Var) ps6127PolicyState {
	if pass == nil || call == nil || regexField == nil || argumentIndex < 0 || argumentIndex >= len(call.Args) {
		return ps6127PolicyUnknown
	}
	callee := ps6071CalledFunction(pass, call)
	if callee == nil {
		return ps6127PolicyUnknown
	}
	declaration := functions[ps6090FunctionID(callee)]
	if declaration == nil || declaration.Body == nil || declaration.Recv != nil || declaration.Type.Params == nil {
		return ps6127PolicyUnknown
	}
	signature, _ := callee.Type().(*types.Signature)
	if signature == nil || signature.Variadic() || argumentIndex >= signature.Params().Len() {
		return ps6127PolicyUnknown
	}
	formal := signature.Params().At(argumentIndex)
	cleared := false
	for _, statement := range declaration.Body.List {
		switch statement := statement.(type) {
		case *ast.EmptyStmt:
			continue
		case *ast.ReturnStmt:
			if len(statement.Results) != 0 {
				return ps6127PolicyUnknown
			}
			if cleared {
				return ps6127PolicyAbsent
			}
			return ps6127PolicyCovered
		case *ast.AssignStmt:
			if ps6127DirectRegexClear(pass, statement, formal, regexField) {
				if cleared {
					return ps6127PolicyUnknown
				}
				cleared = true
				continue
			}
			if cleared || !ps6127ReadOnlyRegexObservation(pass, statement, formal, regexField) {
				return ps6127PolicyUnknown
			}
		default:
			return ps6127PolicyUnknown
		}
	}
	if cleared {
		return ps6127PolicyAbsent
	}
	return ps6127PolicyCovered
}

func ps6127DirectRegexClear(pass *analysis.Pass, assignment *ast.AssignStmt, formal *types.Var, regexField *types.Var) bool {
	if assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	selector, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.SelectorExpr)
	if !ok || !ps6127ExactFormalField(pass, selector, formal, regexField) {
		return false
	}
	id, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.Ident)
	return ok && id.Name == "nil" && pass.TypesInfo.ObjectOf(id) == types.Universe.Lookup("nil")
}

func ps6127ReadOnlyRegexObservation(pass *analysis.Pass, assignment *ast.AssignStmt, formal *types.Var, regexField *types.Var) bool {
	if assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	blank, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || blank.Name != "_" {
		return false
	}
	call, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	builtin, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok || builtin.Name != "len" {
		return false
	}
	object, ok := pass.TypesInfo.ObjectOf(builtin).(*types.Builtin)
	if !ok || object.Name() != "len" {
		return false
	}
	selector, ok := ps2110Unparen(call.Args[0]).(*ast.SelectorExpr)
	return ok && ps6127ExactFormalField(pass, selector, formal, regexField)
}

func ps6127ExactFormalField(pass *analysis.Pass, selector *ast.SelectorExpr, formal *types.Var, regexField *types.Var) bool {
	base, ok := ps2110Unparen(selector.X).(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(base) != formal {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection != nil {
		field, ok := selection.Obj().(*types.Var)
		return ok && field == regexField
	}
	field, _ := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Var)
	return field == regexField
}
