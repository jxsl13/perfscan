package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Receiver storage is bounded to one packed field plus numeric shape values.
// References/caches/interfaces and promoted fields are not source certificates.
func ps6141ReceiverPacked(t types.Type) types.Type {
	if pointer, ok := types.Unalias(t).(*types.Pointer); ok {
		t = pointer.Elem()
	}
	structure, ok := types.Unalias(t).Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	var packed types.Type
	for i := 0; i < structure.NumFields(); i++ {
		field := structure.Field(i)
		if field.Embedded() {
			return nil
		}
		if slice, ok := types.Unalias(field.Type()).Underlying().(*types.Slice); ok {
			if packed != nil || !ps6141SummaryLayout(slice.Elem(), 0) {
				return nil
			}
			packed = field.Type()
		} else if !ps6141NumericScalar(field.Type()) {
			return nil
		}
	}
	return packed
}

func ps6141MethodReceiver(pass *analysis.Pass, decl *ast.FuncDecl, recv *types.Var) bool {
	if ps6141ReceiverPacked(recv.Type()) == nil {
		return false
	}
	immutable := true
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		var targets []ast.Expr
		switch node := n.(type) {
		case *ast.AssignStmt:
			targets = node.Lhs
		case *ast.IncDecStmt:
			targets = []ast.Expr{node.X}
		}
		for _, target := range targets {
			if id, ok := target.(*ast.Ident); ok && identObject(pass, id) == recv {
				immutable = false
			}
		}
		return true
	})
	return immutable
}

// Bind the actual receiver, not a method expression's extra argument or a
// captured method value. Exact types forbid implicit addressing/promotion.
func ps6141ActualReceiver(pass *analysis.Pass, call *ast.CallExpr, sig *types.Signature) ast.Expr {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || sig.Recv() == nil {
		return nil
	}
	selection := pass.TypesInfo.Selections[selector]
	_, direct := ps2110Unparen(selector.X).(*ast.Ident)
	if selection == nil || selection.Kind() != types.MethodVal || len(selection.Index()) != 1 || !direct || !types.Identical(pass.TypesInfo.TypeOf(selector.X), sig.Recv().Type()) {
		return nil
	}
	return selector.X
}
