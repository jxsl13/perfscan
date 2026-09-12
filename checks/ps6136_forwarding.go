package checks

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
)

// The selected F32 projector lowers its exact input/output, active rows and
// source-established stored weight dimensions to the configured recorder API.
func ps6136ProjectionLowering(pass *analysis.Pass, declaration *ast.FuncDecl, methodID string, weight, inner, width *types.Var) bool {
	if pass == nil || declaration == nil || declaration.Body == nil || declaration.Recv == nil || len(declaration.Recv.List) != 1 || len(declaration.Recv.List[0].Names) != 1 || len(declaration.Body.List) != 1 {
		return false
	}
	function, ok := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if !ok {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.Params().Len() != 4 || signature.Results().Len() != 1 || !types.Identical(signature.Results().At(0).Type(), types.Universe.Lookup("error").Type()) || !types.Identical(signature.Params().At(3).Type(), types.Typ[types.Int]) {
		return false
	}
	returned, ok := declaration.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	call, ok := returned.Results[0].(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() || len(call.Args) != 6 {
		return false
	}
	callee, target, ok := typedCallee(pass, call.Fun)
	if !ok || ps6090FunctionID(callee) != methodID || target.Variadic() || target.TypeParams().Len() != 0 || target.Params().Len() != 6 || target.Results().Len() != 1 || !types.Identical(target.Results().At(0).Type(), types.Universe.Lookup("error").Type()) {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[identifier] != signature.Params().At(0) {
		return false
	}
	parameters := [6]int{1, -1, 2, 3, -1, -1}
	fields := [6]*types.Var{nil, weight, nil, nil, inner, width}
	receiver := pass.TypesInfo.Defs[declaration.Recv.List[0].Names[0]]
	for index, argument := range call.Args {
		if !types.Identical(pass.TypesInfo.TypeOf(argument), target.Params().At(index).Type()) {
			return false
		}
		if parameter := parameters[index]; parameter >= 0 {
			identifier, ok := argument.(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[identifier] != signature.Params().At(parameter) {
				return false
			}
			continue
		}
		selector, ok := argument.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		selection := pass.TypesInfo.Selections[selector]
		identifier, ok := selector.X.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != receiver || selection == nil || selection.Kind() != types.FieldVal || selection.Obj() != fields[index] {
			return false
		}
	}
	return true
}

// ps6136ForwardedLeaf proves a direct typed adapter call preserves every
// buffer/geometry argument in order. Native dimension/access semantics and the
// native-recorder factory binding are separate facts, never inferred from a
// method name. The buffer bridge's source identity/ownership must be closed by
// ps6136BufferBridge and constructor result binding, respectively.
func ps6136ForwardedLeaf(pass *analysis.Pass, declaration *ast.FuncDecl, nativeID string, bridge *types.Func, buffers []int) bool {
	if pass == nil || declaration == nil || declaration.Body == nil || declaration.Recv == nil || len(declaration.Recv.List) != 1 || len(declaration.Recv.List[0].Names) != 1 || len(declaration.Body.List) != 1 || bridge == nil {
		return false
	}
	function, ok := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if !ok {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.Results().Len() != 1 || !types.Identical(signature.Results().At(0).Type(), types.Universe.Lookup("error").Type()) {
		return false
	}
	for _, index := range buffers {
		if index < 0 || index >= signature.Params().Len() {
			return false
		}
	}
	returned, ok := declaration.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	call, ok := returned.Results[0].(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() || len(call.Args) != signature.Params().Len() {
		return false
	}
	native, nativeSignature, ok := typedCallee(pass, call.Fun)
	if !ok || ps6090FunctionID(native) != nativeID || nativeSignature.Recv() == nil || nativeSignature.Variadic() || nativeSignature.TypeParams().Len() != 0 || nativeSignature.Params().Len() != len(call.Args) || nativeSignature.Results().Len() != 1 || !types.Identical(nativeSignature.Results().At(0).Type(), types.Universe.Lookup("error").Type()) {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	field, ok := method.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	receiver, ok := field.X.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[receiver] != pass.TypesInfo.Defs[declaration.Recv.List[0].Names[0]] {
		return false
	}
	selection := pass.TypesInfo.Selections[field]
	if selection == nil || selection.Kind() != types.FieldVal || !types.Identical(selection.Type(), nativeSignature.Recv().Type()) {
		return false
	}
	for index, argument := range call.Args {
		if slices.Contains(buffers, index) {
			conversion, ok := argument.(*ast.CallExpr)
			if !ok || conversion.Ellipsis.IsValid() || len(conversion.Args) != 1 {
				return false
			}
			converter, conversionSignature, ok := typedCallee(pass, conversion.Fun)
			if !ok || converter != bridge || conversionSignature.Variadic() || conversionSignature.Params().Len() != 1 || conversionSignature.Results().Len() != 1 || !types.Identical(conversionSignature.Params().At(0).Type(), signature.Params().At(index).Type()) || !types.Identical(conversionSignature.Results().At(0).Type(), nativeSignature.Params().At(index).Type()) {
				return false
			}
			argument = conversion.Args[0]
		} else if !types.Identical(signature.Params().At(index).Type(), types.Typ[types.Int]) || !types.Identical(nativeSignature.Params().At(index).Type(), types.Typ[types.Int]) {
			return false
		}
		identifier, ok := argument.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != signature.Params().At(index) {
			return false
		}
	}
	return true
}

// ps6136BufferBridge proves the source assertion unwraps the exact constructor
// wrapper and native buffer field, without subviews, pointer offsets or helpers.
func ps6136BufferBridge(pass *analysis.Pass, declaration *ast.FuncDecl, wrapper *types.Named, field *types.Var) bool {
	if pass == nil || declaration == nil || declaration.Body == nil || len(declaration.Body.List) != 1 || wrapper == nil || field == nil {
		return false
	}
	function, ok := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if !ok {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() != nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.Params().Len() != 1 || signature.Results().Len() != 1 || !types.Identical(signature.Results().At(0).Type(), field.Type()) {
		return false
	}
	returned, ok := declaration.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	selector, ok := returned.Results[0].(*ast.SelectorExpr)
	if !ok || pass.TypesInfo.Selections[selector] == nil || pass.TypesInfo.Selections[selector].Obj() != field {
		return false
	}
	assertion, ok := selector.X.(*ast.TypeAssertExpr)
	if !ok || assertion.Type == nil || !types.Identical(pass.TypesInfo.TypeOf(assertion.Type), wrapper) {
		return false
	}
	identifier, ok := assertion.X.(*ast.Ident)
	return ok && pass.TypesInfo.Uses[identifier] == signature.Params().At(0)
}
