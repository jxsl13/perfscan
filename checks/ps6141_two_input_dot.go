package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
)

// A complete pure-Go one-dimensional dot, not a Q8_K codec or native summary.
// Equal lengths, a single full packed traversal and matching independent weight
// indices are source obligations; no row spelling or semantic flag supplies them.
func ps6141TwoInputDot(pass *analysis.Pass, fn *ast.FuncDecl, packedType types.Type, c *config.SingleUseQuantizationContract) bool {
	if fn == nil || fn.Recv != nil || fn.Body == nil || len(fn.Body.List) != 4 {
		return false
	}
	object, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if object == nil {
		return false
	}
	sig := object.Type().(*types.Signature)
	if sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Params().Len() != 2 || sig.Results().Len() != 1 ||
		!types.Identical(sig.Results().At(0).Type(), types.Typ[types.Int]) || !types.Identical(sig.Params().At(c.PackedArgument).Type(), packedType) || !types.Identical(sig.Params().At(c.WeightArgument).Type(), packedType) {
		return false
	}
	packed, weights := sig.Params().At(c.PackedArgument), sig.Params().At(c.WeightArgument)
	guard, ok := fn.Body.List[0].(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	condition, ok := guard.Cond.(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ || !ps6141FormalLen(pass, condition.X, packed) || !ps6141FormalLen(pass, condition.Y, weights) {
		return false
	}
	statement, ok := guard.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	panicCall, ok := statement.X.(*ast.CallExpr)
	if !ok || panicCall.Ellipsis.IsValid() || len(panicCall.Args) != 1 {
		return false
	}
	panicID, ok := panicCall.Fun.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[panicID] != types.Universe.Lookup("panic") || pass.TypesInfo.Types[panicCall.Args[0]].Value == nil {
		return false
	}
	initial, ok := fn.Body.List[1].(*ast.AssignStmt)
	if !ok || initial.Tok != token.DEFINE || len(initial.Lhs) != 1 || len(initial.Rhs) != 1 || !ps6141ConstantInt(pass, initial.Rhs[0], 0) {
		return false
	}
	sum, ok := initial.Lhs[0].(*ast.Ident)
	if !ok {
		return false
	}
	sumObject := pass.TypesInfo.Defs[sum]
	loop, ok := fn.Body.List[2].(*ast.RangeStmt)
	if !ok || loop.Tok != token.DEFINE || loop.Value != nil || len(loop.Body.List) != 1 {
		return false
	}
	index, ok := loop.Key.(*ast.Ident)
	if !ok || index.Name == "_" {
		return false
	}
	input, ok := loop.X.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[input] != packed {
		return false
	}
	update, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || update.Tok != token.ADD_ASSIGN || len(update.Lhs) != 1 || len(update.Rhs) != 1 {
		return false
	}
	target, ok := update.Lhs[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[target] != sumObject {
		return false
	}
	product, ok := update.Rhs[0].(*ast.BinaryExpr)
	if !ok || product.Op != token.MUL {
		return false
	}
	convertedIndex := func(expr ast.Expr, formal types.Object) bool {
		call, ok := expr.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
			return false
		}
		cast, ok := call.Fun.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[cast] != types.Universe.Lookup("int") {
			return false
		}
		read, ok := call.Args[0].(*ast.IndexExpr)
		return ok && ps6141Index(pass, read, formal, pass.TypesInfo.Defs[index])
	}
	if !convertedIndex(product.X, packed) || !convertedIndex(product.Y, weights) {
		return false
	}
	returned, ok := fn.Body.List[3].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	result, ok := returned.Results[0].(*ast.Ident)
	return ok && pass.TypesInfo.Uses[result] == sumObject
}

func ps6141FormalLen(pass *analysis.Pass, expr ast.Expr, formal types.Object) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[fn] != types.Universe.Lookup("len") {
		return false
	}
	input, ok := call.Args[0].(*ast.Ident)
	return ok && pass.TypesInfo.Uses[input] == formal
}
