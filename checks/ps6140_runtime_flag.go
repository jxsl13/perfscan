package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Resolve only an actual flag read of this proved constructor's returned owner.
// A method parameter of the same type, or another invocation of the same
// constructor, is not enough. The constructor call must have completed before
// this exact read; constructor-internal observations cannot use final values.
func ps6140RuntimeFlag(proof *ps6140ImmutableFlags, context *ps6125SSAContext, value ssa.Value, ownerType *types.Named) (bool, bool) {
	if proof == nil || proof.snapshot == nil || context == nil || context.flow == nil || value == nil || ownerType == nil {
		return false, false
	}
	snapshot := proof.snapshot
	constructor := snapshot.constructor
	if constructor == nil || constructor.parent == nil || constructor.site == nil {
		return false, false // No actual publication invocation to join.
	}
	reference := context.reference(value)
	load, ok := reference.value.(*ssa.UnOp)
	if !ok || reference.context == nil || load.Op != token.MUL {
		return false, false
	}
	address, ok := load.X.(*ssa.FieldAddr)
	structure, structured := ownerType.Underlying().(*types.Struct)
	if !ok || !structured || !types.Identical(address.X.Type(), types.NewPointer(ownerType)) || address.Field < 0 || address.Field >= structure.NumFields() {
		return false, false
	}
	field := structure.Field(address.Field)
	truth, tracked := snapshot.values[field]
	if !tracked || ps6136OwnerRoot(reference.context, address.X, ownerType) != snapshot.owner {
		return false, false
	}
	for current := reference.context; current != nil; current = current.parent {
		if current == constructor {
			return false, false
		}
	}
	if !ps6136ContextInstructionDominates(constructor.parent, constructor.site, reference.context, load, 256) {
		return false, false
	}
	return truth, true
}
