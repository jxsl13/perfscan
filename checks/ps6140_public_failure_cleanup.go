package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// The private constructor's failure cleanup does not cover an outer wrapper
// discarding a successfully constructed owner. Close every exact nil return
// along its actual public invocation chain. Correlating an outer error check
// with a child's particular failure return is not inferred here: when that
// distinction is needed without an independently ordered cleanup, stay unknown.
func ps6140PublicFailureCleanup(publication, constructor *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, release *types.Func, budget int) bool {
	if constructor == nil || constructor.flow == nil || publication == nil || budget <= 0 {
		return false
	}
	releases, complete := ps6140SourceOwnerReleases(publication, owner, ownerType, release, &budget)
	if !complete {
		return false
	}
	for child := constructor; child != publication; child = child.parent {
		context := child.parent
		if context == nil || context.flow == nil || child.site == nil || context.calls[child.site] != child {
			return false
		}
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				budget--
				if budget <= 0 {
					return false
				}
				returned, ok := instruction.(*ssa.Return)
				if !ok || len(returned.Results) == 0 {
					continue
				}
				value, nilOwner := returned.Results[0].(*ssa.Const)
				if !nilOwner || !value.IsNil() {
					continue
				}
				if !types.Identical(value.Type(), types.NewPointer(ownerType)) {
					return false
				}
				if !ps6136InstructionCanPrecede(context, child.site, returned) {
					continue // No selected owner has been constructed on this path.
				}
				cleaned := false
				for _, released := range releases {
					budget--
					if budget <= 0 {
						return false
					}
					if ps6136ContextInstructionDominates(context, child.site, released.context, released.call, budget) &&
						ps6136ContextInstructionDominates(released.context, released.call, context, returned, budget) {
						cleaned = true
						break
					}
				}
				if !cleaned {
					return false
				}
			}
		}
	}
	return budget > 0
}
