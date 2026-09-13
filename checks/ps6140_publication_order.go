package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Unlike unconditional call completion, successful owner publication excludes
// a factory's nil/error returns. Lift only the actual returned-owner call chain;
// every nonnil return must publish this same fresh owner and satisfy the order.
// This does not prove an instruction ran on failed construction, or permit
// arbitrary post-call observations to assume successful construction.
func ps6140PublicationInstructionDominates(from *ps6125SSAContext, instruction ssa.Instruction, to *ps6125SSAContext, returned *ssa.Return, owner ps6125SSAReference, ownerType *types.Named, budget int) bool {
	if from == nil || instruction == nil || to == nil || returned == nil || returned.Parent() != to.flow.function || len(returned.Results) == 0 || owner.value == nil || ownerType == nil || budget <= 0 || ps6136OwnerRoot(to, returned.Results[0], ownerType) != owner {
		return false
	}
	if ps6136ContextInstructionDominates(from, instruction, to, returned, budget) {
		return true
	}
	call, ok := returned.Results[0].(*ssa.Call)
	result := 0
	if extract, extracted := returned.Results[0].(*ssa.Extract); extracted {
		call, ok = extract.Tuple.(*ssa.Call)
		result = extract.Index
	}
	if !ok || result != 0 || !to.flow.instructionDominates(call, returned) {
		return false
	}
	child := ps6136Call(to, call)
	if child == nil {
		return false
	}
	publications := 0
	for _, block := range child.flow.function.Blocks {
		if !child.flow.blocks[block] {
			continue
		}
		for _, candidate := range block.Instrs {
			budget--
			if budget <= 0 {
				return false
			}
			next, ok := candidate.(*ssa.Return)
			if !ok {
				continue
			}
			if len(next.Results) == 0 {
				return false
			}
			if value, ok := next.Results[0].(*ssa.Const); ok && value.IsNil() {
				continue
			}
			publications++
			if !ps6140PublicationInstructionDominates(from, instruction, child, next, owner, ownerType, budget) {
				return false
			}
		}
	}
	return publications != 0
}
