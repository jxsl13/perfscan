package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6140BlockMember binds an actual runtime value to an indexed member of this
// owner's exact block slice. It does not prove its index is in range or that
// collection initialization/mutation is closed; those are independent gates.
func ps6140BlockMember(context *ps6125SSAContext, value ssa.Value, owner ps6125SSAReference, ownerType, blockType *types.Named, blocks *types.Var, remaining int) bool {
	if context == nil || context.flow == nil || value == nil || owner.value == nil || ownerType == nil || blockType == nil || blocks == nil || remaining <= 0 {
		return false
	}
	reference := context.reference(value)
	if reference.value == nil {
		return false
	}
	if reference.context != context || reference.value != value {
		return ps6140BlockMember(reference.context, reference.value, owner, ownerType, blockType, blocks, remaining-1)
	}
	if !types.Identical(value.Type(), blockType) {
		return false
	}
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	cell := context.reference(load.X)
	if allocation, ok := cell.value.(*ssa.Alloc); ok {
		stores := cell.context.structCell(allocation)
		if stores == nil || stores.whole == nil {
			return false
		}
		var point ssa.Instruction = load
		current := context
		for current != cell.context && current.parent != nil {
			point = current.site
			current = current.parent
		}
		if current != cell.context || !cell.context.flow.instructionDominates(stores.whole, point) {
			return false
		}
		return ps6140BlockMember(cell.context, stores.whole.Val, owner, ownerType, blockType, blocks, remaining-1)
	}
	index, ok := load.X.(*ssa.IndexAddr)
	if !ok || !types.Identical(index.X.Type(), types.NewSlice(blockType)) {
		return false
	}
	paths := ps6125AccessPaths{flow: context.flow}
	path := paths.resolve(index.X)
	return path.known && len(path.access.fields) == 1 && path.access.fields[0] == blocks && ps6136AccessOwnerRoot(context, path, ownerType) == owner
}
