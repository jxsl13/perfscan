package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136ConstructorErrorBarrier proves the canonical final publication edge:
// the selected constructor returns this owner only with nil aggregate error;
// the opposite edge calls its exact Release before returning nil/error. This
// does not infer Release effects, prior error-cell invariance, allocator failure
// ownership, or the completeness of earlier construction returns.
func ps6136ConstructorErrorBarrier(context *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, cell ps6125SSAReference, release string) bool {
	if context == nil || owner.value == nil || cell.value == nil || ownerType == nil {
		return false
	}
	var published *ssa.Return
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			ret, ok := instruction.(*ssa.Return)
			if !ok || len(ret.Results) != 2 {
				continue
			}
			if literal, ok := ret.Results[0].(*ssa.Const); ok && literal.IsNil() {
				continue
			}
			if published != nil || ps6136OwnerRoot(context, ret.Results[0], ownerType) != owner {
				return false
			}
			literal, ok := ret.Results[1].(*ssa.Const)
			if !ok || !literal.IsNil() {
				return false
			}
			published = ret
		}
	}
	if published == nil || len(published.Block().Preds) != 1 {
		return false
	}
	// No allocation, write or unknown effect may occur after checking the
	// aggregate error and before publication. Only the exact returned owner
	// and same error cell's effect-free pointer loads are permitted.
	for _, instruction := range published.Block().Instrs {
		switch use := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.UnOp:
			if use.Op != token.MUL || ps6136OwnerRoot(context, use, ownerType) != owner && !ps6136ErrorCellRead(context, use, cell) {
				return false
			}
		case *ssa.Return:
			if use != published {
				return false
			}
		default:
			return false
		}
	}
	predecessor := published.Block().Preds[0]
	if len(predecessor.Instrs) == 0 || len(predecessor.Succs) != 2 {
		return false
	}
	branch, ok := predecessor.Instrs[len(predecessor.Instrs)-1].(*ssa.If)
	if !ok {
		return false
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false
	}
	value, nilValue := comparison.X, comparison.Y
	if literal, ok := value.(*ssa.Const); ok && literal.IsNil() {
		value, nilValue = nilValue, value
	}
	literal, ok := nilValue.(*ssa.Const)
	if !ok || !literal.IsNil() || !ps6136ErrorCellRead(context, value, cell) {
		return false
	}
	checked := context.reference(value)
	load, ok := checked.value.(*ssa.UnOp)
	if !ok || checked.context != context || load.Block() != predecessor || comparison.Block() != predecessor {
		return false
	}
	// The predicate must use the current error, not a snapshot taken before
	// another allocation or error write. Close the entire read-to-branch
	// interval, including precomputed boolean predicates.
	reading := false
	for _, instruction := range predecessor.Instrs {
		if instruction == load {
			reading = true
			continue
		}
		if !reading {
			continue
		}
		switch instruction {
		case comparison, branch:
		default:
			if _, debug := instruction.(*ssa.DebugRef); !debug {
				return false
			}
		}
	}
	if !reading {
		return false
	}
	errorIndex := 1
	if comparison.Op == token.NEQ {
		errorIndex = 0
	}
	if predecessor.Succs[1-errorIndex] != published.Block() {
		return false
	}
	failure := predecessor.Succs[errorIndex]
	var cleanup *ssa.Call
	var returned *ssa.Return
	for _, instruction := range failure.Instrs {
		switch use := instruction.(type) {
		case *ssa.Call:
			function := use.Call.StaticCallee()
			if function == nil || function.Object() == nil || use.Call.IsInvoke() || len(use.Call.Args) != 1 || cleanup != nil {
				return false
			}
			object, ok := function.Object().(*types.Func)
			if !ok || ps6090FunctionID(object) != release || ps6136OwnerRoot(context, use.Call.Args[0], ownerType) != owner {
				return false
			}
			cleanup = use
		case *ssa.Return:
			if returned != nil || len(use.Results) != 2 {
				return false
			}
			literal, ok := use.Results[0].(*ssa.Const)
			if !ok || !literal.IsNil() || !ps6136ErrorCellRead(context, use.Results[1], cell) {
				return false
			}
			returned = use
		case *ssa.DebugRef:
		case *ssa.UnOp:
			if use.Op != token.MUL || ps6136OwnerRoot(context, use, ownerType) != owner && !ps6136ErrorCellRead(context, use, cell) {
				return false
			}
		default:
			return false
		}
	}
	return cleanup != nil && returned != nil && context.flow.instructionDominates(cleanup, returned)
}

func ps6136ErrorCellRead(context *ps6125SSAContext, value ssa.Value, cell ps6125SSAReference) bool {
	reference := context.reference(value)
	load, ok := reference.value.(*ssa.UnOp)
	return ok && load.Op == token.MUL && reference.context.reference(load.X) == cell
}
