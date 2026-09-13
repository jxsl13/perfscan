package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// This combines a selected publication snapshot with package-wide flag and
// owner-effect closure. It does not associate an arbitrary runtime receiver
// with that constructor: consumers must still join the exact owner lifetime.
type ps6140ImmutableFlags struct {
	snapshot *ps6140ConstructorFlagSnapshot
}

func ps6140ImmutableConstructorFlags(pass *analysis.Pass, pkg *ssa.Package, snapshot *ps6140ConstructorFlagSnapshot, owner *types.Named, budget int) *ps6140ImmutableFlags {
	return ps6140ImmutableConstructorFlagsCheck(pass, pkg, snapshot, owner, budget, nil)
}

func ps6140ImmutableConstructorFlagsCheck(pass *analysis.Pass, pkg *ssa.Package, snapshot *ps6140ConstructorFlagSnapshot, owner *types.Named, budget int, rejected func(string)) (result *ps6140ImmutableFlags) {
	stage := "input"
	defer func() {
		if result == nil && rejected != nil {
			rejected(stage)
		}
	}()
	if pass == nil || pass.TypesInfo == nil || pass.Pkg == nil || pkg == nil || pass.Pkg != pkg.Pkg || owner == nil || snapshot == nil || snapshot.constructor == nil || snapshot.constructor.flow == nil || snapshot.constructor.flow.function.Pkg != pkg || snapshot.owner.value == nil || len(snapshot.values) == 0 || budget <= 0 {
		return nil
	}
	structure, ok := owner.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	for field := range snapshot.values {
		if field == nil || field.Exported() || ps6136FieldVar(owner, field.Name()) != field || !types.Identical(field.Type(), types.Typ[types.Bool]) {
			return nil // Outside-package writes to exported flags are not closed.
		}
	}
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.Contains(comment.Text, "go:linkname") {
					return nil
				}
			}
		}
	}
	stage = "constructor inventory"
	inventory := ps6136ConstructorInventory(pkg, owner)
	if inventory == nil || inventory[snapshot.constructor.flow.function].value == nil {
		return nil
	}
	// Only direct writes in functions that themselves return this fresh owner
	// qualify. A helper also called by a constructor is not thereby a safe
	// post-publication setter; such helpers require a separate call-use proof.
	positions := make(map[token.Pos]*types.Var)
	for _, function := range ps6136SourceFunctions(pkg) {
		stage = "source stores " + function.String()
		context := inventory[function].context
		for context != nil && context.flow.function != function {
			context = context.parent
		}
		if context == nil {
			context = ps6125NewSSAContext(function, nil, nil, 16384)
		}
		if context == nil || context.flow == nil {
			return nil // An unanalyzed body cannot disappear from the census.
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				budget--
				if budget <= 0 {
					return nil
				}
				store, ok := instruction.(*ssa.Store)
				if !ok {
					continue
				}
				if types.Identical(store.Addr.Type(), types.NewPointer(owner)) {
					return nil // Replacement invalidates flags without a selector.
				}
				address, ok := store.Addr.(*ssa.FieldAddr)
				if !ok || !types.Identical(address.X.Type(), types.NewPointer(owner)) || address.Field < 0 || address.Field >= structure.NumFields() {
					continue
				}
				field := structure.Field(address.Field)
				if _, tracked := snapshot.values[field]; !tracked {
					continue
				}
				fresh := inventory[function]
				if fresh.value == nil || ps6136OwnerRoot(context, address.X, owner) != fresh || !address.Pos().IsValid() {
					return nil
				}
				positions[address.Pos()] = field
			}
		}
	}
	proved := ps6136OwnerTransfers(pass, pkg, owner)
	if proved == nil {
		return nil
	}
	for field := range snapshot.values {
		for node := range ps6136InteriorOwnerCells(pass, pkg, owner, field) {
			proved[node] = true
		}
	}
	initializers := make(map[ast.Node]bool)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.SelectorExpr:
				selection := pass.TypesInfo.Selections[node]
				if selection == nil || selection.Kind() != types.FieldVal {
					break
				}
				field, _ := selection.Obj().(*types.Var)
				if _, tracked := snapshot.values[field]; tracked {
					proved[node] = true // Scalar reads; writes/addresses gated below.
					if positions[node.Sel.Pos()] == field {
						initializers[node] = true
					}
				}
			case *ast.KeyValueExpr:
				key, ok := node.Key.(*ast.Ident)
				if !ok {
					break
				}
				field, _ := pass.TypesInfo.Uses[key].(*types.Var)
				for position, stored := range positions {
					if stored == field && node.Pos() <= position && position < node.End() {
						initializers[node] = true
						proved[key] = true
					}
				}
			}
			return true
		})
	}
	for field := range snapshot.values {
		stage = "field effects " + field.Name()
		if !ps6136FieldEffects(pass, field, initializers) {
			return nil
		}
		stage = "owner observations " + field.Name()
		if !ps6136ClosedObservationsCheck(pass, owner, field, proved, func(node ast.Node, reason string) {
			stage += ": " + reason + " " + pass.Fset.Position(node.Pos()).String()
		}) {
			return nil
		}
	}
	return &ps6140ImmutableFlags{snapshot: snapshot}
}
