package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Exact instance/workspace/slot-buffer identity through source interface
// capability assertions. No slice offsets, returned opaque aliases, loads from
// unrelated owners or boolean assertion results are interpreted as a buffer.
func ps6136BufferOrigin(context *ps6125SSAContext, value ssa.Value, owner ps6125SSAReference, ownerType *types.Named, workspace, slot *types.Var, budget int) bool {
	if context == nil || value == nil || owner.value == nil || ownerType == nil || workspace == nil || slot == nil || budget <= 0 {
		return false
	}
	reference := context.reference(value)
	if reference.value == nil {
		return false
	}
	if reference.context != context || reference.value != value {
		return ps6136BufferOrigin(reference.context, reference.value, owner, ownerType, workspace, slot, budget-1)
	}
	paths := ps6125AccessPaths{flow: context.flow}
	path := paths.resolve(value)
	if path.known && len(path.access.fields) == 2 && path.access.fields[0] == workspace && path.access.fields[1] == slot && ps6136AccessOwnerRoot(context, path, ownerType) == owner {
		return true
	}
	switch source := value.(type) {
	case *ssa.TypeAssert:
		return !source.CommaOk && ps6136BufferOrigin(context, source.X, owner, ownerType, workspace, slot, budget-1)
	case *ssa.Extract:
		assertion, ok := source.Tuple.(*ssa.TypeAssert)
		return ok && assertion.CommaOk && source.Index == 0 && ps6136BufferOrigin(context, assertion.X, owner, ownerType, workspace, slot, budget-1)
	case *ssa.ChangeInterface:
		return ps6136BufferOrigin(context, source.X, owner, ownerType, workspace, slot, budget-1)
	case *ssa.Phi:
		found := false
		for index, predecessor := range source.Block().Preds {
			if !context.flow.edges[ps6125SSAEdge{from: predecessor, to: source.Block()}] {
				continue
			}
			if !ps6136BufferOrigin(context, source.Edges[index], owner, ownerType, workspace, slot, budget-1) {
				return false
			}
			found = true
		}
		return found
	}
	return false
}
