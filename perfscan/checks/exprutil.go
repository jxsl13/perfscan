package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// pkgRefCount counts identifiers in file that resolve to the named
// package import — the guard fixes use to avoid orphaning an import by
// rewriting a file's last reference to it.
func pkgRefCount(pass *analysis.Pass, file *ast.File, path string) int {
	n := 0
	ast.Inspect(file, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if pn, ok := pass.TypesInfo.Uses[id].(*types.PkgName); ok && pn.Imported().Path() == path {
			n++
		}
		return true
	})
	return n
}

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

// enclosingMethodRecv returns the receiver identifier name and method name
// of the *ast.FuncDecl whose body lexically encloses target, when that
// FuncDecl is a method with a NAMED receiver. ok is false otherwise (free
// function, unnamed or blank receiver, or no enclosing declaration). Go
// never nests FuncDecls and closures are *ast.FuncLit, so at most one
// FuncDecl encloses target — a closure inside a method still resolves to
// that method (it closes over the receiver).
func enclosingMethodRecv(pass *analysis.Pass, target ast.Node) (recvName, methodName string, ok bool) {
	for _, f := range pass.Files {
		if target.Pos() < f.Pos() || target.End() > f.End() {
			continue
		}
		for _, d := range f.Decls {
			fn, isFn := d.(*ast.FuncDecl)
			if !isFn || fn.Body == nil ||
				fn.Body.Pos() > target.Pos() || target.End() > fn.Body.End() {
				continue
			}
			if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
				return "", "", false
			}
			name := fn.Recv.List[0].Names[0].Name
			if name == "_" {
				return "", "", false
			}
			return name, fn.Name.Name, true
		}
		return "", "", false
	}
	return "", "", false
}

// writeFixSelfDispatches reports whether rewriting a write whose writer
// expression is writer would dispatch back into the method that lexically
// encloses node — i.e. the "fix" would turn a correct delegation (like
// WriteString(s) { return d.Write([]byte(s)) }) into unbounded recursion.
// dispatch is the set of method names the rewritten call lands on (e.g.
// "WriteString", or "Write" for the fmt.Fprintf family). It triggers only
// when writer is a bare identifier naming the enclosing method's receiver
// and that method's name is in dispatch; a selector like d.buf or any other
// expression is a different object and is safe. When this reports true the
// finding must be suppressed entirely: the original code is the correct
// delegation and no valid rewrite exists.
func writeFixSelfDispatches(pass *analysis.Pass, node ast.Node, writer ast.Expr, dispatch ...string) bool {
	id, isIdent := writer.(*ast.Ident)
	if !isIdent {
		return false
	}
	recvName, methodName, ok := enclosingMethodRecv(pass, node)
	if !ok || id.Name != recvName {
		return false
	}
	for _, m := range dispatch {
		if methodName == m {
			return true
		}
	}
	return false
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
