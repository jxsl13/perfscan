package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// ps6136OwnerTransfers inventories only direct source calls that forward an
// exact owner parameter/receiver, and fresh-constructor publication returns.
// Approvals cover just the owner operand, never a whole call/function. This
// closes no field effects or buffer extents by itself: the complete package's
// alias/effect/consumer gates remain mandatory.
func ps6136OwnerTransfers(pass *analysis.Pass, pkg *ssa.Package, owner *types.Named) map[ast.Node]bool {
	if pass == nil || pkg == nil || owner == nil {
		return nil
	}
	constructors := ps6136ConstructorInventory(pkg, owner)
	if constructors == nil {
		return nil
	}
	proved := make(map[ast.Node]bool)
	isOwner := func(typ types.Type) bool {
		return types.Identical(typ, owner) || types.Identical(typ, types.NewPointer(owner))
	}
	// Never approve a compound operand: an outer source call/return must not
	// hide an opaque escape or workspace read inside its argument expression.
	simple := func(expression ast.Expr) bool {
		_, identifier := ps2110Unparen(expression).(*ast.Ident)
		return identifier
	}
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || call.Ellipsis.IsValid() {
				return true
			}
			function, signature, ok := typedCallee(pass, call.Fun)
			if !ok || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
				return true
			}
			callee := pkg.Prog.FuncValue(function)
			if callee == nil || callee.Pkg != pkg || len(callee.Blocks) == 0 || callee.Syntax() == nil {
				return true // No opaque owner-transfer semantics inferred.
			}
			arguments := call.Args
			if signature.Recv() != nil && isOwner(signature.Recv().Type()) {
				selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
				if !ok {
					return true
				}
				selection := pass.TypesInfo.Selections[selector]
				if selection == nil {
					return true
				}
				switch selection.Kind() {
				case types.MethodVal:
					if isOwner(pass.TypesInfo.TypeOf(selector.X)) && simple(selector.X) {
						proved[selector.X] = true
					}
				case types.MethodExpr:
					if len(arguments) == 0 || !isOwner(pass.TypesInfo.TypeOf(arguments[0])) || !simple(arguments[0]) {
						return true
					}
					proved[arguments[0]] = true
					arguments = arguments[1:]
				default:
					return true
				}
			}
			if len(arguments) != signature.Params().Len() {
				return true
			}
			for index, argument := range arguments {
				if isOwner(signature.Params().At(index).Type()) && types.Identical(pass.TypesInfo.TypeOf(argument), signature.Params().At(index).Type()) && simple(argument) {
					proved[argument] = true
				}
			}
			return true
		})
	}
	for function := range constructors {
		syntax := function.Syntax()
		if syntax == nil {
			return nil
		}
		var body *ast.BlockStmt
		switch declaration := syntax.(type) {
		case *ast.FuncDecl:
			body = declaration.Body
		case *ast.FuncLit:
			body = declaration.Body
		}
		if body == nil {
			return nil
		}
		ast.Inspect(body, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			if returned, ok := node.(*ast.ReturnStmt); ok && len(returned.Results) > 0 && isOwner(pass.TypesInfo.TypeOf(returned.Results[0])) && simple(returned.Results[0]) {
				proved[returned.Results[0]] = true
			}
			return true
		})
	}
	return proved
}
