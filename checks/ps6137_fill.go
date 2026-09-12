package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
)

func ps6137Fill(pass *analysis.Pass, f, own ps6137Function, c *config.NativeSnapshotReuseContract, result types.Type, slice *types.Slice) bool {
	s := f.sig
	if s.Recv() != nil || s.TypeParams().Len() != 0 || s.Variadic() || s.Params().Len() != 3 || s.Results().Len() != 0 || len(f.decl.Body.List) != 2 {
		return false
	}
	if c.ResultSliceField == "" {
		if !types.Identical(s.Params().At(0).Type(), result) {
			return false
		}
	} else {
		p, ok := types.Unalias(s.Params().At(0).Type()).(*types.Pointer)
		if !ok || !types.Identical(p.Elem(), result) {
			return false
		}
	}
	native, ok := types.Unalias(s.Params().At(1).Type()).Underlying().(*types.Slice)
	if !ok {
		return false
	}
	if _, ok := types.Unalias(native.Elem()).Underlying().(*types.Struct); !ok {
		return false
	}
	tokens, ok := types.Unalias(s.Params().At(2).Type()).Underlying().(*types.Slice)
	if !ok || !ps6137Integer(tokens.Elem()) {
		return false
	}
	decl, ok := f.decl.Body.List[0].(*ast.DeclStmt)
	if !ok {
		return false
	}
	gen, ok := decl.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR || len(gen.Specs) != 1 {
		return false
	}
	spec, ok := gen.Specs[0].(*ast.ValueSpec)
	if !ok || len(spec.Names) != 1 || len(spec.Values) != 0 {
		return false
	}
	cache := ps6137Local(pass, spec.Names[0])
	if cache == nil {
		return false
	}
	cachePointer, ok := types.Unalias(own.sig.Recv().Type()).(*types.Pointer)
	if !ok || !types.Identical(cache.Type(), cachePointer.Elem()) {
		return false
	}
	loop, ok := f.decl.Body.List[1].(*ast.RangeStmt)
	if !ok || loop.Tok != token.DEFINE || loop.Value != nil || !ps6137Events(pass, loop.X, s.Params().At(0), c) {
		return false
	}
	index := ps6137Local(pass, loop.Key)
	if index == nil {
		return false
	}
	want := 2
	if c.EventSpanField != "" {
		want = 4
	}
	if len(loop.Body.List) != want {
		return false
	}
	bind, ok := ps6137Assignment(loop.Body.List[0])
	if !ok {
		return false
	}
	event := ps6137Local(pass, bind.Lhs[0])
	address, ok := ps2110Unparen(bind.Rhs[0]).(*ast.UnaryExpr)
	if !ok || address.Op != token.AND {
		return false
	}
	slot, ok := ps2110Unparen(address.X).(*ast.IndexExpr)
	if !ok || !ps6135Object(pass, slot.X, s.Params().At(1)) || !ps6135Object(pass, slot.Index, index) || event == nil {
		return false
	}
	write, ok := loop.Body.List[1].(*ast.AssignStmt)
	if !ok || write.Tok != token.ASSIGN || len(write.Lhs) != 1 || len(write.Rhs) != 1 {
		return false
	}
	destination, ok := ps2110Unparen(write.Lhs[0]).(*ast.IndexExpr)
	if !ok || !ps6137Events(pass, destination.X, s.Params().At(0), c) || !ps6135Object(pass, destination.Index, index) {
		return false
	}
	literal, ok := ps2110Unparen(write.Rhs[0]).(*ast.CompositeLit)
	if !ok || !types.Identical(pass.TypesInfo.TypeOf(literal), slice.Elem()) {
		return false
	}
	if !ps6137EventLiteral(pass, literal, event, c, func(e ast.Expr) bool {
		call, ok := ps2110Unparen(e).(*ast.CallExpr)
		if !ok || len(call.Args) != 2 || ps6137CallID(pass, call) != c.OwnMethod || ps6091MethodExpression(pass, call.Fun) {
			return false
		}
		method, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		if !ok || !ps6135Object(pass, method.X, cache) || !ps6137LabelPointer(pass, call.Args[1], event, c) {
			return false
		}
		convert, ok := ps2110Unparen(call.Args[0]).(*ast.CallExpr)
		if !ok || len(convert.Args) != 1 || !pass.TypesInfo.Types[convert.Fun].IsType() || !types.Identical(pass.TypesInfo.TypeOf(convert), types.Typ[types.Uintptr]) {
			return false
		}
		token, ok := ps2110Unparen(convert.Args[0]).(*ast.IndexExpr)
		return ok && ps6135Object(pass, token.X, s.Params().At(2)) && ps6135Object(pass, token.Index, index)
	}) {
		return false
	}
	if c.EventSpanField != "" && !ps6137FillSpan(pass, loop.Body.List[2:], s.Params().At(0), index, c) {
		return false
	}
	return true
}

func ps6137FillSpan(pass *analysis.Pass, list []ast.Stmt, result, index types.Object, c *config.NativeSnapshotReuseContract) bool {
	bind, ok := ps6137Assignment(list[0])
	if !ok {
		return false
	}
	end := ps6137Local(pass, bind.Lhs[0])
	if end == nil {
		return false
	}
	add, ok := ps2110Unparen(bind.Rhs[0]).(*ast.BinaryExpr)
	if !ok || add.Op != token.ADD {
		return false
	}
	field := func(e ast.Expr, name string) bool {
		selector, ok := ps2110Unparen(e).(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != name {
			return false
		}
		slot, ok := ps2110Unparen(selector.X).(*ast.IndexExpr)
		return ok && ps6137Events(pass, slot.X, result, c) && ps6135Object(pass, slot.Index, index)
	}
	if !field(add.X, c.EventStartField) || !field(add.Y, c.EventDurationField) {
		return false
	}
	span := func(e ast.Expr) bool {
		s, ok := ps2110Unparen(e).(*ast.SelectorExpr)
		return ok && s.Sel.Name == c.EventSpanField && ps6135Object(pass, s.X, result)
	}
	branch, ok := list[1].(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	greater, ok := ps2110Unparen(branch.Cond).(*ast.BinaryExpr)
	if !ok || greater.Op != token.GTR || !ps6135Object(pass, greater.X, end) || !span(greater.Y) {
		return false
	}
	write, ok := branch.Body.List[0].(*ast.AssignStmt)
	return ok && write.Tok == token.ASSIGN && len(write.Lhs) == 1 && len(write.Rhs) == 1 && span(write.Lhs[0]) && ps6135Object(pass, write.Rhs[0], end)
}
