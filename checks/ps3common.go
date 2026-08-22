package checks

// Shared machinery of the PS3xxx serial-nest family (fanOutHelpers domain).

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
)

// packageHasFanOut reports whether any file declares or calls a configured
// fan-out helper.
func packageHasFanOut(pass *analysis.Pass, fanout map[string]bool) bool {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fanout[fn.Name.Name] || callsFanOut(fn.Body, fanout) {
				return true
			}
		}
	}
	return false
}

// nestWrites collects the index expressions of every indexed write
// (assignment LHS) inside a nest.
func nestWrites(nest ast.Node) []*ast.IndexExpr {
	var writes []*ast.IndexExpr
	ast.Inspect(nest, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range as.Lhs {
			if ix, ok := lhs.(*ast.IndexExpr); ok {
				writes = append(writes, ix)
			}
		}
		return true
	})
	return writes
}

// derivedFromVar collects locals assigned (within nest) from expressions
// mentioning v — the obase := b*stride shape.
func derivedFromVar(nest ast.Node, v string) map[string]bool {
	derived := map[string]bool{v: true}
	// Two passes to follow chains assigned in order.
	for range 2 {
		ast.Inspect(nest, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, rhs := range as.Rhs {
				if i >= len(as.Lhs) || !mentionsAnyOf(rhs, derived) {
					continue
				}
				if id, ok := as.Lhs[i].(*ast.Ident); ok && id.Name != "_" {
					derived[id.Name] = true
				}
			}
			return true
		})
	}
	return derived
}

// outermostLoopVar returns the loop variable of a for/range statement.
func outermostLoopVar(n ast.Node) string {
	v, _ := loopVarAndBody(n)
	return v
}

// topLevelNests calls fn for every outermost loop in body whose own body
// contains another loop (a nest root), skipping nested roots.
func topLevelNests(body *ast.BlockStmt, fn func(nest ast.Node)) {
	ast.Inspect(body, func(n ast.Node) bool {
		if !astutil.IsLoop(n) {
			return true
		}
		b := astutil.LoopBody(n)
		if b == nil || !containsLoop(b) {
			return true
		}
		fn(n)
		return false
	})
}
