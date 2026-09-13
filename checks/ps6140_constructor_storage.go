package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Close direct constructor observations from the actual public entry through
// every resolved source invocation, without publication-time flag pruning.
// The only selected field access is its proved initialization; the fresh host
// slice and returned slot must each have exactly their one intended use.
// Native allocator argument/result closure and retained-list observations are
// separate obligations: this gate grants no native freshness or lifetime fact.
func ps6140ConstructorStorage(publication *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, workspace *types.Var, allocation *ps6136ConstructorProof, budget int) bool {
	return ps6140ConstructorStorageCheck(publication, owner, ownerType, workspace, allocation, budget, nil)
}

func ps6140ConstructorStorageCheck(publication *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, workspace *types.Var, allocation *ps6136ConstructorProof, budget int, rejected func(string)) (result bool) {
	stage := "input"
	defer func() {
		if !result && rejected != nil {
			rejected(stage)
		}
	}()
	if publication == nil || publication.flow == nil || owner.context == nil || ownerType == nil || workspace == nil || allocation == nil || allocation.allocation == nil || allocation.factory == nil || budget <= 0 {
		return false
	}
	fresh, ok := owner.value.(*ssa.Alloc)
	if !ok || !types.Identical(fresh.Type(), types.NewPointer(ownerType)) || ps6136FieldVar(ownerType, workspace.Name()) != workspace {
		return false
	}
	factory := allocation.factory
	call, context, store := factory.site, factory.parent, allocation.allocation
	if call == nil || context == nil || context.calls[call] != factory || store.Parent() != context.flow.function || store.Val != call || len(call.Call.Args) != 1 {
		return false
	}
	stage = "fresh host slice and returned slot uses"
	input := context.reference(call.Call.Args[0])
	slice, ok := input.value.(*ssa.MakeSlice)
	if !ok || input.context != context || !types.Identical(slice.Type(), types.NewSlice(types.Typ[types.Float32])) || !ps6140OnlyConstructorUse(context, slice, call, &budget) || !ps6140OnlyConstructorUse(context, call, store, &budget) {
		return false
	}
	structure := ownerType.Underlying().(*types.Struct)
	pending := []*ps6125SSAContext{publication}
	seen := make(map[*ps6125SSAContext]bool)
	initializations := 0
	for len(pending) != 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[current] {
			continue
		}
		seen[current] = true
		for _, block := range current.flow.function.Blocks {
			if !current.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				stage = current.flow.function.String() + ": " + instruction.String()
				budget--
				if budget <= 0 {
					return false
				}
				switch instruction := instruction.(type) {
				case *ssa.Call:
					if child := ps6136Call(current, instruction); child != nil {
						pending = append(pending, child)
					} else if callee := instruction.Call.StaticCallee(); callee != nil && callee.Pkg == publication.flow.function.Pkg && len(callee.Blocks) != 0 {
						return false // Recursion/budget loss cannot hide a local body.
					}
				case *ssa.UnOp:
					if instruction.Op == token.MUL && types.Identical(instruction.Type(), ownerType) {
						return false // A whole-owner copy also copies the slot pointer.
					}
				case *ssa.Field:
					if types.Identical(instruction.X.Type(), ownerType) && instruction.Field >= 0 && instruction.Field < structure.NumFields() && structure.Field(instruction.Field) == workspace {
						return false
					}
				case *ssa.FieldAddr:
					if !types.Identical(instruction.X.Type(), types.NewPointer(ownerType)) || instruction.Field < 0 || instruction.Field >= structure.NumFields() || structure.Field(instruction.Field) != workspace {
						continue
					}
					// An unknown same-type owner may alias the selected instance.
					if current != context || ps6136OwnerRoot(current, instruction.X, ownerType) != owner || store.Addr != instruction || !ps6140OnlyConstructorUse(current, instruction, store, &budget) {
						return false
					}
					initializations++
				}
			}
		}
	}
	return seen[owner.context] && seen[factory] && initializations == 1
}

func ps6140OnlyConstructorUse(context *ps6125SSAContext, value ssa.Value, wanted ssa.Instruction, budget *int) bool {
	if context == nil || value == nil || wanted == nil || value.Referrers() == nil || budget == nil {
		return false
	}
	found := false
	for _, user := range *value.Referrers() {
		*budget--
		if *budget <= 0 || user.Parent() != context.flow.function {
			return false
		}
		if !context.flow.blocks[user.Block()] {
			continue
		}
		if _, debug := user.(*ssa.DebugRef); debug {
			continue
		}
		if user != wanted {
			return false
		}
		found = true
	}
	return found
}
