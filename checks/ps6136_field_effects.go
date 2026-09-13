package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// ps6136FieldEffects closes direct writes/addresses of one exact typed field.
// Only separately proved fresh-constructor initialization targets may be
// allowed. This is not owner-alias closure: ps6136ClosedObservations must also
// reject indirect receiver/holder/opaque mutation paths. A write to a sibling
// child member is not a whole-field write (ProfileMetalStep's recorder changes
// must not falsely imply its allocator field changed).
func ps6136FieldEffects(pass *analysis.Pass, field *types.Var, initializers map[ast.Node]bool) bool {
	if pass == nil || field == nil {
		return false
	}
	parents := make(map[ast.Node]ast.Node)
	var occurrences []ast.Node
	for _, file := range pass.Files {
		var stack []ast.Node
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]
				return false
			}
			if len(stack) > 0 {
				parents[node] = stack[len(stack)-1]
			}
			stack = append(stack, node)
			if selector, ok := node.(*ast.SelectorExpr); ok {
				if selection := pass.TypesInfo.Selections[selector]; selection != nil && selection.Obj() == field {
					occurrences = append(occurrences, selector)
				}
			}
			if pair, ok := node.(*ast.KeyValueExpr); ok {
				if key, ok := pair.Key.(*ast.Ident); ok && pass.TypesInfo.Uses[key] == field {
					occurrences = append(occurrences, pair)
				}
			}
			return true
		})
	}
	for _, occurrence := range occurrences {
		if initializers[occurrence] {
			continue
		}
		if _, literal := occurrence.(*ast.KeyValueExpr); literal {
			return false
		}
		target := occurrence
		// An interior address of a child member also exposes the containing
		// field's storage; sibling assignments alone do not.
		for child := occurrence; child != nil; {
			parent := parents[child]
			if address, ok := parent.(*ast.UnaryExpr); ok && address.Op == token.AND && address.X == child {
				return false
			}
			switch parent := parent.(type) {
			case *ast.ParenExpr:
				child = parent
			case *ast.SelectorExpr:
				if parent.X != child {
					child = nil
				} else {
					child = parent
				}
			default:
				child = nil
			}
		}
		for {
			parens, ok := parents[target].(*ast.ParenExpr)
			if !ok {
				break
			}
			target = parens
		}
		switch parent := parents[target].(type) {
		case *ast.UnaryExpr:
			if parent.Op == token.AND && parent.X == target {
				return false
			}
		case *ast.AssignStmt:
			for _, left := range parent.Lhs {
				if left == target {
					return false
				}
			}
		case *ast.IncDecStmt:
			if parent.X == target {
				return false
			}
		case *ast.RangeStmt:
			if parent.Tok == token.ASSIGN && (parent.Key == target || parent.Value == target) {
				return false
			}
		}
	}
	return true
}
