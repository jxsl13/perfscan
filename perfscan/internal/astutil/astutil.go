// Package astutil provides small AST helpers shared by perfscan checks.
package astutil

import (
	"go/ast"
	"go/types"
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
// a loop's init/cond/post or range expression is not "in" the loop, and a
// function literal is a boundary: code inside a closure is not lexically on
// the enclosing loop's iteration path.
func InLoop(stack []ast.Node) (ast.Node, bool) {
	for i := len(stack) - 1; i >= 1; i-- {
		if _, isLit := stack[i-1].(*ast.FuncLit); isLit {
			return nil, false
		}
		if IsLoop(stack[i-1]) {
			if body := LoopBody(stack[i-1]); body != nil && stack[i] == ast.Node(body) {
				return stack[i-1], true
			}
		}
	}
	return nil, false
}

// OutermostLoop returns the outermost enclosing loop (by body) on the
// stack, honoring the same function-literal boundary as InLoop.
func OutermostLoop(stack []ast.Node) (ast.Node, bool) {
	boundary := 0
	for i := len(stack) - 1; i >= 0; i-- {
		if _, isLit := stack[i].(*ast.FuncLit); isLit {
			boundary = i + 1
			break
		}
	}
	for i := boundary + 1; i < len(stack); i++ {
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
// (nil set matches any), returning the name. The qualifier must resolve —
// via info — to the imported package whose path is pkg (a *types.PkgName).
// A var/func/type/const named like the package does not match; in
// particular a PACKAGE-LEVEL shadow declared in another file leaves
// Ident.Obj nil, so only type resolution discriminates it from the real
// import.
func PkgFuncCall(info *types.Info, fun ast.Expr, pkg string, set map[string]bool) (string, bool) {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != pkg {
		return "", false
	}
	pn, ok := info.Uses[id].(*types.PkgName)
	if !ok {
		return "", false
	}
	// Defensive: the qualifier must be THE stdlib package, not e.g. a
	// third-party package imported under the same name. All callers pass a
	// single-segment stdlib name, for which name == import path.
	if pn.Imported().Path() != pkg {
		return "", false
	}
	if set == nil || set[sel.Sel.Name] {
		return sel.Sel.Name, true
	}
	return "", false
}
