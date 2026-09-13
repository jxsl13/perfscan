package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6140AppendedBlock proves the source's one-element append descriptor and
// actual block value. Publication, collection invariance and consumer index
// ownership must still be closed independently.
func ps6140AppendedBlock(context *ps6125SSAContext, call *ssa.Call, blockType *types.Named) ps6125SSAReference {
	if context == nil || context.flow == nil || call == nil || blockType == nil || call.Parent() != context.flow.function || !context.flow.blocks[call.Block()] || len(call.Call.Args) != 2 {
		return ps6125SSAReference{}
	}
	builtin, ok := call.Call.Value.(*ssa.Builtin)
	if !ok || builtin.Name() != "append" {
		return ps6125SSAReference{}
	}
	slice, ok := call.Call.Args[1].(*ssa.Slice)
	if !ok {
		return ps6125SSAReference{}
	}
	array, ok := slice.X.(*ssa.Alloc)
	if !ok || array.Referrers() == nil {
		return ps6125SSAReference{}
	}
	var value ssa.Value
	for _, user := range *array.Referrers() {
		address, ok := user.(*ssa.IndexAddr)
		if !ok || address.Referrers() == nil {
			continue
		}
		for _, use := range *address.Referrers() {
			if store, ok := use.(*ssa.Store); ok && store.Addr == address {
				if value != nil || !types.Identical(store.Val.Type(), blockType) {
					return ps6125SSAReference{}
				}
				value = store.Val
			}
		}
	}
	if value == nil || ps6136SingleAppend(context, slice, call, value) == nil {
		return ps6125SSAReference{}
	}
	return ps6125SSAReference{context: context, value: value}
}

func ps6140BlockProjectionValue(block ps6125SSAReference, field *types.Var, concrete *types.Named, budget int) bool {
	if block.context == nil || block.value == nil || field == nil || concrete == nil {
		return false
	}
	structure, ok := block.value.Type().Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for index := 0; index < structure.NumFields(); index++ {
		if structure.Field(index) != field {
			continue
		}
		value := ps6140ClosedBlockField(block, index, budget)
		return value.value != nil && ps6140ConcreteValue(value.context, value.value, concrete, budget)
	}
	return false
}
