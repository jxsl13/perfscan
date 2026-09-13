package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Map actual source store positions to exact initialization targets, never to
// a function/block-wide allowance that could conceal contradictory extra uses.
func ps6136ConstructorWriteNodes(pass *analysis.Pass, pkg *ssa.Package, owner *types.Named, fields map[*types.Var]bool, writes *ps6136ConstructorWrites) map[ast.Node]bool {
	if pass == nil || writes == nil {
		return nil
	}
	functions := ps6136SourceFunctions(pkg)
	constructors := ps6136ConstructorInventory(pkg, owner)
	if constructors == nil {
		return nil
	}
	for writer := range writes.writers {
		object, ok := writer.Object().(*types.Func)
		if !ok || !object.Exported() || constructors[writer].value != nil {
			continue
		}
		// Internal construction calls cannot establish that an exported
		// setter is constructor-only; clients may invoke it on a published
		// instance. Actual output/geometry initializing helpers are private.
		return nil
	}
	proved := make(map[ast.Node]bool)
	valid := true
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if !valid {
				return false
			}
			var target ast.Node
			switch occurrence := node.(type) {
			case *ast.SelectorExpr:
				if selection := pass.TypesInfo.Selections[occurrence]; selection != nil {
					if field, ok := selection.Obj().(*types.Var); ok && fields[field] {
						target = occurrence
					}
				}
			case *ast.KeyValueExpr:
				if key, ok := occurrence.Key.(*ast.Ident); ok {
					if field, ok := pass.TypesInfo.Uses[key].(*types.Var); ok && fields[field] {
						target = occurrence
					}
				}
			case *ast.CallExpr:
				function, _, ok := typedCallee(pass, occurrence.Fun)
				if !ok {
					break
				}
				callee := pkg.Prog.FuncValue(function)
				if !writes.writers[callee] || constructors[callee].value != nil {
					break
				}
				// A constructor-only mutating helper cannot later be invoked
				// by an inference/wrapper method on a published owner.
				var caller *ssa.Function
				for _, candidate := range functions {
					syntax := candidate.Syntax()
					if syntax != nil && syntax.Pos() <= occurrence.Pos() && occurrence.End() <= syntax.End() && (caller == nil || caller.Syntax().Pos() < syntax.Pos()) {
						caller = candidate
					}
				}
				if caller == nil || !writes.contexts[caller] {
					valid = false
				}
			}
			if target != nil {
				for position := range writes.positions {
					if target.Pos() <= position && position < target.End() {
						proved[target] = true
					}
				}
			}
			return valid
		})
	}
	if !valid {
		return nil
	}
	return proved
}
