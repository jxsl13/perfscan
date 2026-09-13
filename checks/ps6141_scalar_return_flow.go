package checks

import (
	"go/ast"
	"go/types"
)

// Conservative returned use of immutable scalar origins. Effect/completion
// proof is prerequisite; mutable locals are unknown, not stale dependencies.
// Every syntactic return must retain an origin; no control-dependency guess.
func (index *ps6141SummaryIndex) immutableScalarReturns(fn *types.Func) ps6141SourceSummary {
	effect := ps6141ScalarEffects(index.pass, fn)
	if !effect.effectsKnown || !effect.completionKnown {
		return ps6141SourceSummary{}
	}
	decl := index.declarations[fn.Origin()]
	if decl == nil {
		return ps6141SourceSummary{}
	}
	sig := fn.Type().(*types.Signature)
	initializers := map[types.Object]ast.Expr{}
	mutable := map[types.Object]bool{}
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					if object := index.pass.TypesInfo.Uses[id]; object != nil {
						mutable[object] = true
					} else if len(node.Lhs) == 1 && len(node.Rhs) == 1 {
						initializers[index.pass.TypesInfo.Defs[id]] = node.Rhs[0]
					}
				}
			}
		case *ast.IncDecStmt:
			if id, ok := node.X.(*ast.Ident); ok {
				mutable[identObject(index.pass, id)] = true
			}
		}
		return true
	})
	body := &ps6141SummaryBody{index: index, summary: ps6141SourceSummary{valid: true}, roots: map[types.Object]ps6141Root{}, scalars: map[types.Object]ps6141Deps{}, contributions: map[types.Object]ps6141Deps{}, laneContributions: map[types.Object]ps6141Deps{}, callResults: map[*ast.CallExpr]ps6141SourceSummary{}, suppressedValues: map[types.Object]bool{}}
	for _, object := range index.pass.TypesInfo.Defs {
		if object != nil && object.Pos() >= decl.Pos() && object.Pos() < decl.End() && ps6141NumericScalar(object.Type()) {
			body.scalars[object] = nil
		}
	}
	for at := 0; at < sig.Params().Len(); at++ {
		param := sig.Params().At(at)
		if !mutable[param] {
			body.scalars[param] = ps6141Deps{ps6141Root(at + 1): true}
		}
	}
	// Lexical initializer order is important; this is not iterative dependency
	// accumulation. Go forbids forward local uses except closures (already denied).
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		assignment, ok := n.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		id, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		object := index.pass.TypesInfo.Defs[id]
		if initializer := initializers[object]; object != nil && !mutable[object] && initializer != nil {
			body.scalars[object] = body.expression(initializer)
			body.suppressedValues[object] = body.erases(initializer)
		}
		return true
	})
	var returned ps6141Deps
	first := true
	suppressed := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		statement, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(statement.Results) != 1 {
			body.summary.valid = false
			return true
		}
		deps := body.expression(statement.Results[0])
		suppressed = suppressed || body.erases(statement.Results[0])
		if first {
			returned = ps6141Deps{}
			for root := range deps {
				returned[root] = true
			}
			first = false
		} else {
			// Symbolic-map domain: returned origins are a sparse subset of arbitrary formal positions, not a dense traversal index
			for root := range returned {
				if !deps[root] { //perfscan:ignore PS3003 returned formal origins are a sparse subset of arbitrary argument positions, not dense traversal indexes
					delete(returned, root)
				}
			}
		}
		return true
	})
	if first || !body.summary.valid {
		return ps6141SourceSummary{}
	}
	return ps6141SourceSummary{valid: true, resultDeps: returned, resultSuppressed: suppressed}
}
