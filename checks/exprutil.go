package checks

import (
	"go/ast"
)

// baseIdentName returns the root identifier name of x, x.f, x[i], (x), &x.
func baseIdentName(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x.Name
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.UnaryExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		default:
			return ""
		}
	}
}

// exprMentions reports whether e references the identifier name.
func exprMentions(e ast.Expr, name string) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

// exprEqual compares two expressions by their rendered text.
func exprEqual(a, b ast.Expr) bool {
	return exprTextRendered(a) == exprTextRendered(b)
}

// isSliceMake reports whether call is make([]T, ...).
func isSliceMake(call *ast.CallExpr) bool {
	if astCalleeName(call) != "make" || len(call.Args) == 0 {
		return false
	}
	at, ok := call.Args[0].(*ast.ArrayType)
	return ok && at.Len == nil
}

func astCalleeName(call *ast.CallExpr) string {
	if id, ok := call.Fun.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// isPointerMethod reports whether fn is a method with a pointer receiver.
func isPointerMethod(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return false
	}
	_, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	return ok
}

// collectEscaping returns the local names that outlive a loop iteration:
// returned, stored by reference into a field/slot (recv.f = x, ring[i] = x,
// m[k] = x), or used as a value in a composite literal.
func collectEscaping(body *ast.BlockStmt) map[string]bool {
	escaping := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ReturnStmt:
			for _, r := range x.Results {
				if id, ok := r.(*ast.Ident); ok {
					escaping[id.Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				switch lhs.(type) {
				case *ast.SelectorExpr, *ast.IndexExpr, *ast.StarExpr:
					if i < len(x.Rhs) {
						if id, ok := x.Rhs[i].(*ast.Ident); ok {
							escaping[id.Name] = true
						}
					}
				}
			}
		case *ast.CompositeLit:
			for _, elt := range x.Elts {
				switch e := elt.(type) {
				case *ast.KeyValueExpr:
					if id, ok := e.Value.(*ast.Ident); ok {
						escaping[id.Name] = true
					}
				case *ast.Ident:
					escaping[e.Name] = true
				}
			}
		}
		return true
	})
	return escaping
}
