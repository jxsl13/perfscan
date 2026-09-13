package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// ps6136ClosedObservations requires a source proof for every workspace field
// observation and every whole-owner exposure in the loaded package. A witness
// in a selected helper cannot establish complete use coverage. Proved nodes
// are supplied only after exact typed argument, extent and ownership checks;
// this routine never interprets configuration as a source proof.
func ps6136ClosedObservations(pass *analysis.Pass, owner *types.Named, workspace *types.Var, proved map[ast.Node]bool) bool {
	return ps6136ClosedObservationsCheck(pass, owner, workspace, proved, nil)
}

// The optional diagnostic reports the first rejected source node; it grants no
// approval and preserves the strict observation checks used by the rule.
func ps6136ClosedObservationsCheck(pass *analysis.Pass, owner *types.Named, workspace *types.Var, proved map[ast.Node]bool, rejected func(ast.Node, string)) bool {
	if owner == nil || workspace == nil {
		return false
	}
	parents := make(map[ast.Node]ast.Node)
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
			return true
		})
	}
	approved := func(node ast.Node) bool {
		for node != nil {
			if proved[node] {
				return true
			}
			// A function-wide approval would hide contradictory extra uses.
			switch node.(type) {
			case *ast.FuncDecl, *ast.FuncLit, *ast.BlockStmt:
				return false
			}
			node = parents[node]
		}
		return false
	}
	isOwner := func(typ types.Type) bool {
		if pointer, ok := typ.(*types.Pointer); ok {
			typ = pointer.Elem()
		}
		return typ != nil && types.Identical(typ, owner)
	}
	valid := true
	reject := func(node ast.Node, reason string) {
		if !valid {
			return
		}
		valid = false
		if rejected != nil {
			rejected(node, reason)
		}
	}
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if !valid {
				return false
			}
			switch n := node.(type) {
			case *ast.UnaryExpr:
				if n.Op == token.AND && !approved(n) {
					// An interior struct-field pointer can expose owner storage
					// without passing an expression typed *Owner. Slice indexes
					// refer to separate backing storage and are not inferred here.
					for expression := ps2110Unparen(n.X); expression != nil; {
						selector, ok := expression.(*ast.SelectorExpr)
						if !ok {
							break
						}
						if isOwner(pass.TypesInfo.TypeOf(selector.X)) {
							reject(n, "interior owner field address")
							break
						}
						expression = ps2110Unparen(selector.X)
					}
				}
			case *ast.SelectorExpr:
				selection := pass.TypesInfo.Selections[n]
				if selection != nil && selection.Obj() == workspace && !approved(n) {
					reject(n, "unproved workspace selector")
				}
				if isOwner(pass.TypesInfo.TypeOf(n)) && !approved(n) {
					outer, ok := parents[n].(*ast.SelectorExpr)
					if !ok || outer.X != n {
						reject(n, "whole owner selector exposure")
					} else if next := pass.TypesInfo.Selections[outer]; next == nil || next.Kind() != types.FieldVal {
						reject(n, "owner selector method exposure")
					}
				}
			case *ast.CompositeLit:
				if isOwner(pass.TypesInfo.TypeOf(n)) && !approved(n) {
					for _, element := range n.Elts {
						keyed, ok := element.(*ast.KeyValueExpr)
						if !ok {
							reject(n, "unkeyed owner literal")
							break
						}
						identifier, ok := keyed.Key.(*ast.Ident)
						if !ok || pass.TypesInfo.Uses[identifier] == workspace && !proved[keyed.Key] {
							reject(n, "unproved owner literal workspace")
							break
						}
					}
				}
			case *ast.Ident:
				object := pass.TypesInfo.Uses[n]
				if _, variable := object.(*types.Var); !variable || !isOwner(object.Type()) || approved(n) {
					break
				}
				parent := parents[n]
				for {
					parens, ok := parent.(*ast.ParenExpr)
					if !ok {
						break
					}
					parent = parents[parens]
				}
				if comparison, ok := parent.(*ast.BinaryExpr); ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) {
					other := comparison.X
					if ps2110Unparen(comparison.X) == n {
						other = comparison.Y
					}
					if identifier, ok := ps2110Unparen(other).(*ast.Ident); ok && pass.TypesInfo.Uses[identifier] == types.Universe.Lookup("nil") {
						break // Pointer nil comparison neither aliases nor exposes storage.
					}
				}
				selector, ok := parent.(*ast.SelectorExpr)
				if !ok {
					reject(n, "whole owner identifier exposure") // aliases, captures, sends, opaque arguments, whole stores
					break
				}
				selection := pass.TypesInfo.Selections[selector]
				if selection == nil || selection.Kind() != types.FieldVal {
					reject(n, "implicit owner method exposure") // implicit-address methods require their exact call proof
				}
			}
			return valid
		})
	}
	return valid
}
