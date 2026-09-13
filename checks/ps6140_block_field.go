package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Project one field from a private block value even when sibling fields have
// conditional initialization. This does not relax the general struct-cell
// proof: all address uses are closed, the selected field is initialized once,
// and its exact store (or whole-value copy) must dominate this value load.
func ps6140ClosedBlockField(block ps6125SSAReference, index, budget int) ps6125SSAReference {
	if block.context == nil || block.value == nil || budget <= 0 {
		return ps6125SSAReference{}
	}
	if value := block.context.structField(block.value, index); value.value != nil {
		return value
	}
	load, ok := block.value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL || load.Parent() != block.context.flow.function {
		return ps6125SSAReference{}
	}
	allocation, ok := load.X.(*ssa.Alloc)
	structure, isStruct := load.Type().Underlying().(*types.Struct)
	if !ok || !isStruct || index < 0 || index >= structure.NumFields() || allocation.Parent() != load.Parent() ||
		allocation.Referrers() == nil || !block.context.flow.instructionDominates(allocation, load) {
		return ps6125SSAReference{}
	}
	var whole, selected *ssa.Store
	for _, instruction := range *allocation.Referrers() {
		budget--
		if budget <= 0 || instruction.Parent() != load.Parent() {
			return ps6125SSAReference{}
		}
		if !block.context.flow.blocks[instruction.Block()] {
			continue
		}
		switch user := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.UnOp:
			if user != load {
				return ps6125SSAReference{}
			}
		case *ssa.Store:
			if user.Addr != allocation || whole != nil || !types.Identical(user.Val.Type(), load.Type()) {
				return ps6125SSAReference{}
			}
			whole = user
		case *ssa.FieldAddr:
			if user.X != allocation || user.Field < 0 || user.Field >= structure.NumFields() || user.Referrers() == nil {
				return ps6125SSAReference{}
			}
			for _, access := range *user.Referrers() {
				budget--
				if budget <= 0 || access.Parent() != load.Parent() {
					return ps6125SSAReference{}
				}
				if !block.context.flow.blocks[access.Block()] {
					continue
				}
				switch operation := access.(type) {
				case *ssa.DebugRef:
				case *ssa.UnOp:
					if operation.Op != token.MUL || operation.X != user {
						return ps6125SSAReference{}
					}
				case *ssa.Store:
					if operation.Addr != user {
						return ps6125SSAReference{}
					}
					if user.Field == index {
						if selected != nil {
							return ps6125SSAReference{}
						}
						selected = operation
					}
				default:
					return ps6125SSAReference{} // including sibling address escape
				}
			}
		default:
			return ps6125SSAReference{} // captures, aliases, calls, returns, go/defer
		}
	}
	if whole != nil {
		if selected != nil || !block.context.flow.instructionDominates(whole, load) {
			return ps6125SSAReference{}
		}
		return ps6140ClosedBlockField(block.context.reference(whole.Val), index, budget-1)
	}
	if selected == nil || !block.context.flow.instructionDominates(selected, load) {
		return ps6125SSAReference{}
	}
	return block.context.reference(selected.Val)
}
