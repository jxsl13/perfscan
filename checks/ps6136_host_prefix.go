package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// ps6136HostPrefix distinguishes an unbounded physical ToHost transfer from
// its source-visible first-width logical consumption. It accepts only the
// actual closed Tensor.Storage().F32() chain and canonical prefix copy; extra
// tensor/slice observations, aliases or full consumers are unknown, not small.
func ps6136HostPrefix(pass *analysis.Pass, transfer *ast.CallExpr, receiver types.Object, width *types.Var, storage, f32 *types.Func) bool {
	if pass == nil || transfer == nil || receiver == nil || width == nil || storage == nil || f32 == nil {
		return false
	}
	parents := make(map[ast.Node]ast.Node)
	for _, file := range pass.Files {
		var stack []ast.Node
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			if len(stack) > 0 {
				parents[node] = stack[len(stack)-1]
			}
			stack = append(stack, node)
			return true
		})
	}
	assignment, ok := parents[transfer].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Rhs) != 1 || len(assignment.Lhs) != 2 {
		return false
	}
	tensor, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Defs[tensor] == nil {
		return false
	}
	tensorObject := pass.TypesInfo.Defs[tensor]
	var tensorUse *ast.Ident
	for identifier, object := range pass.TypesInfo.Uses {
		if object != tensorObject {
			continue
		}
		if tensorUse != nil {
			return false
		}
		tensorUse = identifier
	}
	selector, ok := parents[tensorUse].(*ast.SelectorExpr)
	if !ok || selector.X != tensorUse || !ps6136SelectedMethod(pass, selector, storage) {
		return false
	}
	storageCall, ok := parents[selector].(*ast.CallExpr)
	if !ok || storageCall.Fun != selector || len(storageCall.Args) != 0 {
		return false
	}
	f32Selector, ok := parents[storageCall].(*ast.SelectorExpr)
	if !ok || f32Selector.X != storageCall || !ps6136SelectedMethod(pass, f32Selector, f32) {
		return false
	}
	f32Call, ok := parents[f32Selector].(*ast.CallExpr)
	if !ok || f32Call.Fun != f32Selector || len(f32Call.Args) != 0 || !types.Identical(pass.TypesInfo.TypeOf(f32Call), types.NewSlice(types.Typ[types.Float32])) {
		return false
	}
	sliceAssignment, ok := parents[f32Call].(*ast.AssignStmt)
	if !ok || sliceAssignment.Tok != token.DEFINE || len(sliceAssignment.Lhs) != 1 || len(sliceAssignment.Rhs) != 1 {
		return false
	}
	slice, ok := sliceAssignment.Lhs[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Defs[slice] == nil {
		return false
	}
	var sliceUse *ast.Ident
	for identifier, object := range pass.TypesInfo.Uses {
		if object == pass.TypesInfo.Defs[slice] {
			if sliceUse != nil {
				return false
			}
			sliceUse = identifier
		}
	}
	index, ok := parents[sliceUse].(*ast.IndexExpr)
	if !ok || index.X != sliceUse {
		return false
	}
	var loop *ast.ForStmt
	for node := ast.Node(index); node != nil; node = parents[node] {
		if candidate, ok := node.(*ast.ForStmt); ok {
			loop = candidate
			break
		}
	}
	if loop == nil || len(loop.Body.List) != 1 {
		return false
	}
	init, ok := loop.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return false
	}
	i, ok := init.Lhs[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Defs[i] == nil {
		return false
	}
	zero := pass.TypesInfo.Types[init.Rhs[0]].Value
	if zero == nil || zero.Kind() != constant.Int || constant.Sign(zero) != 0 {
		return false
	}
	condition, ok := loop.Cond.(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS || !ps6136ObjectExpr(pass, condition.X, pass.TypesInfo.Defs[i]) || !ps6136DirectField(pass, condition.Y, receiver, width) {
		return false
	}
	post, ok := loop.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC || !ps6136ObjectExpr(pass, post.X, pass.TypesInfo.Defs[i]) || !ps6136ObjectExpr(pass, index.Index, pass.TypesInfo.Defs[i]) {
		return false
	}
	copy, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || copy.Tok != token.ASSIGN || len(copy.Lhs) != 1 || len(copy.Rhs) != 1 {
		return false
	}
	destination, ok := copy.Lhs[0].(*ast.IndexExpr)
	if !ok || !ps6136ObjectExpr(pass, destination.Index, pass.TypesInfo.Defs[i]) {
		return false
	}
	// A call/container expression on the assignment's left side may mutate
	// i before the source index is evaluated. The actual owner copies to a
	// bare []float64 variable; no effectful destination grammar is approved.
	destinationBuffer, ok := destination.X.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[destinationBuffer] == nil || !types.Identical(pass.TypesInfo.TypeOf(destinationBuffer), types.NewSlice(types.Typ[types.Float64])) {
		return false
	}
	conversion, ok := copy.Rhs[0].(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 || conversion.Args[0] != index || !types.Identical(pass.TypesInfo.TypeOf(conversion.Fun), types.Typ[types.Float64]) {
		return false
	}
	allowedIteratorUses := map[ast.Node]bool{condition.X: true, post.X: true, index.Index: true, destination.Index: true}
	for identifier, object := range pass.TypesInfo.Uses {
		if object == pass.TypesInfo.Defs[i] && !allowedIteratorUses[identifier] {
			return false
		}
	}
	return true
}

func ps6136SelectedMethod(pass *analysis.Pass, selector *ast.SelectorExpr, method *types.Func) bool {
	selection := pass.TypesInfo.Selections[selector]
	return selection != nil && selection.Kind() == types.MethodVal && selection.Obj() == method
}
