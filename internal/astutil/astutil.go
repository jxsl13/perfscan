// Package astutil provides small AST helpers shared by perfscan checks.
package astutil

import (
	"go/ast"
)

// WithStack walks root and calls fn for every non-nil node with the stack of
// its ancestors (excluding the node itself, innermost last). Return false to
// skip the node's children.
func WithStack(root ast.Node, fn func(n ast.Node, stack []ast.Node) bool) {
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		descend := fn(n, stack)
		if descend {
			stack = append(stack, n)
		}
		return descend
	})
}

// LoopBody returns the body of a for/range statement, or nil when n is not a
// loop.
func LoopBody(n ast.Node) *ast.BlockStmt {
	switch l := n.(type) {
	case *ast.ForStmt:
		return l.Body
	case *ast.RangeStmt:
		return l.Body
	}
	return nil
}

// IsLoop reports whether n is a for or range statement.
func IsLoop(n ast.Node) bool {
	switch n.(type) {
	case *ast.ForStmt, *ast.RangeStmt:
		return true
	}
	return false
}

// InLoop reports whether the stack contains a for/range loop whose BODY
// encloses the current node, and returns the innermost such loop. A node in
// a loop's init/cond/post or range expression is not "in" the loop.
func InLoop(stack []ast.Node) (ast.Node, bool) {
	for i := len(stack) - 1; i >= 1; i-- {
		if IsLoop(stack[i-1]) {
			if body := LoopBody(stack[i-1]); body != nil && stack[i] == ast.Node(body) {
				return stack[i-1], true
			}
		}
	}
	return nil, false
}

// OutermostLoop returns the outermost enclosing loop (by body) on the stack.
func OutermostLoop(stack []ast.Node) (ast.Node, bool) {
	for i := 1; i < len(stack); i++ {
		if IsLoop(stack[i-1]) {
			if body := LoopBody(stack[i-1]); body != nil && stack[i] == ast.Node(body) {
				return stack[i-1], true
			}
		}
	}
	return nil, false
}

// CalleeName returns the bare name of a call target: "f" for f(...),
// "m" for x.m(...), "" otherwise.
func CalleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// PkgFuncCall reports whether fun is a selector pkg.Name with Name in set
// (nil set matches any), returning the name.
func PkgFuncCall(fun ast.Expr, pkg string, set map[string]bool) (string, bool) {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != pkg {
		return "", false
	}
	// A local object named like the package shadows it.
	if id.Obj != nil {
		return "", false
	}
	if set == nil || set[sel.Sel.Name] {
		return sel.Sel.Name, true
	}
	return "", false
}
