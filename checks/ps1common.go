package checks

// Shared machinery of the PS1xxx per-element-access family. The vocabulary
// (element accessors, count methods, index decomposers, fast-path helpers)
// comes from the perfscan.yaml config — see the config package.

import (
	"go/ast"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
)

// funcFacts holds the per-function facts the PS1xxx checks share.
type funcFacts struct {
	// hasFlat: the function calls a configured typed bulk accessor
	// (fastPathHelpers). Its presence means a per-element loop is only
	// the strided/other-dtype fallback, not the hot path.
	hasFlat bool
	// numelIdents: locals bound to an element count (n := t.Numel()) or a
	// shape dimension (d := t.Shape()[1]).
	numelIdents map[string]bool
	// perElemLoop classifies each loop as a full-tensor walk.
	perElemLoop map[ast.Node]bool
	// parent maps every node to its parent for ancestor walks.
	parent map[ast.Node]ast.Node
}

func collectFuncFacts(fn *ast.FuncDecl, ns *config.Sets) *funcFacts {
	ff := &funcFacts{
		numelIdents: map[string]bool{},
		perElemLoop: map[ast.Node]bool{},
		parent:      map[ast.Node]ast.Node{},
	}
	var stack []ast.Node
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			ff.parent[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		switch x := n.(type) {
		case *ast.CallExpr:
			if ns.FastPathHelpers[astutil.CalleeName(x.Fun)] {
				ff.hasFlat = true
			}
		case *ast.AssignStmt:
			for i, rhs := range x.Rhs {
				if i >= len(x.Lhs) {
					break
				}
				id, ok := x.Lhs[i].(*ast.Ident)
				if !ok {
					continue
				}
				// n := t.Numel()
				if call, ok := rhs.(*ast.CallExpr); ok && ns.ElementCountMethods[astutil.CalleeName(call.Fun)] {
					ff.numelIdents[id.Name] = true
				}
				// d := t.Shape()[1] — walks d elements of that axis
				if ix, ok := rhs.(*ast.IndexExpr); ok {
					if call, ok := ix.X.(*ast.CallExpr); ok && ns.ShapeMethods[astutil.CalleeName(call.Fun)] {
						ff.numelIdents[id.Name] = true
					}
				}
			}
		}
		return true
	})

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch loop := n.(type) {
		case *ast.RangeStmt:
			ff.perElemLoop[loop] = isNumelRange(loop, ff.numelIdents, ns) || directlyHasUnravel(loop.Body, ns)
		case *ast.ForStmt:
			ff.perElemLoop[loop] = isNumelForCond(loop, ff.numelIdents, ns) || directlyHasUnravel(loop.Body, ns)
		}
		return true
	})
	return ff
}

// isNumelRange reports a range over an element-count call or a local bound
// to one.
func isNumelRange(r *ast.RangeStmt, numelIdents map[string]bool, ns *config.Sets) bool {
	switch x := r.X.(type) {
	case *ast.CallExpr:
		return ns.ElementCountMethods[astutil.CalleeName(x.Fun)]
	case *ast.Ident:
		return numelIdents[x.Name]
	}
	return false
}

// isNumelForCond reports a 3-clause for loop bounded by an element count:
// `for i := 0; i < n; i++` with n from a configured count method.
func isNumelForCond(f *ast.ForStmt, numelIdents map[string]bool, ns *config.Sets) bool {
	bin, ok := f.Cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	for _, side := range []ast.Expr{bin.X, bin.Y} {
		if id, ok := side.(*ast.Ident); ok && numelIdents[id.Name] {
			return true
		}
		if call, ok := side.(*ast.CallExpr); ok && ns.ElementCountMethods[astutil.CalleeName(call.Fun)] {
			return true
		}
	}
	return false
}

// directlyHasUnravel reports whether a loop body calls a configured
// flat-index decomposer in ITS OWN scope. It deliberately does not descend
// into a nested Range/For/FuncLit: an Unravel there belongs to that inner
// loop, so counting it would misclassify an outer per-row loop as
// per-element.
func directlyHasUnravel(body *ast.BlockStmt, ns *config.Sets) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n.(type) {
		case *ast.RangeStmt, *ast.ForStmt, *ast.FuncLit:
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok && ns.IndexDecomposeFuncs[astutil.CalleeName(call.Fun)] {
			found = true
			return false
		}
		return true
	})
	return found
}

// nearestLoop walks parent links to the innermost enclosing for/range loop.
func nearestLoop(parent map[ast.Node]ast.Node, n ast.Node) ast.Node {
	for p := parent[n]; p != nil; p = parent[p] {
		if astutil.IsLoop(p) {
			return p
		}
	}
	return nil
}

// anyAncestorPerElem reports whether any enclosing loop is per-element.
func anyAncestorPerElem(parent map[ast.Node]ast.Node, perElem map[ast.Node]bool, n ast.Node) bool {
	for p := parent[n]; p != nil; p = parent[p] {
		if perElem[p] {
			return true
		}
	}
	return false
}

// enclosingLoopVars returns the iteration-variable names of every loop
// enclosing n.
func enclosingLoopVars(parent map[ast.Node]ast.Node, n ast.Node) map[string]bool {
	vars := map[string]bool{}
	for p := parent[n]; p != nil; p = parent[p] {
		switch l := p.(type) {
		case *ast.RangeStmt:
			if id, ok := l.Key.(*ast.Ident); ok && id.Name != "_" {
				vars[id.Name] = true
			}
			if id, ok := l.Value.(*ast.Ident); ok && id.Name != "_" {
				vars[id.Name] = true
			}
		case *ast.ForStmt:
			if as, ok := l.Init.(*ast.AssignStmt); ok {
				for _, lhs := range as.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
						vars[id.Name] = true
					}
				}
			}
		}
	}
	return vars
}
