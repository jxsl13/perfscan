package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136ProjectorHeadWidth walks only source returns and closed value structs.
// Every executable alternative must carry the same selected head width. A
// configured projector type does not attest arbitrary opaque factory results.
func ps6136ProjectorHeadWidth(context *ps6125SSAContext, value ssa.Value, projectors map[*types.Named]*types.Var, model ps6125SSAReference, modelType *types.Named, head *types.Var, shape string, axis, budget int) bool {
	if context == nil || value == nil || budget <= 0 {
		return false
	}
	reference := context.reference(value)
	if reference.value == nil {
		return false
	}
	if reference.context != context || reference.value != value {
		return ps6136ProjectorHeadWidth(reference.context, reference.value, projectors, model, modelType, head, shape, axis, budget-1)
	}
	if boxed, ok := value.(*ssa.MakeInterface); ok {
		return ps6136ProjectorHeadWidth(context, boxed.X, projectors, model, modelType, head, shape, axis, budget-1)
	}
	if call, ok := value.(*ssa.Call); ok {
		child := ps6136ZeroSourceFlow(ps6136Call(context, call))
		if child == nil {
			return false
		}
		found := false
		for _, block := range child.flow.function.Blocks {
			if !child.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				ret, ok := instruction.(*ssa.Return)
				if !ok {
					continue
				}
				if len(ret.Results) != 1 || !ps6136ProjectorHeadWidth(child, ret.Results[0], projectors, model, modelType, head, shape, axis, budget-1) {
					return false
				}
				found = true
			}
		}
		return found
	}
	if phi, ok := value.(*ssa.Phi); ok {
		found := false
		for index, predecessor := range phi.Block().Preds {
			if !context.flow.edges[ps6125SSAEdge{from: predecessor, to: phi.Block()}] {
				continue
			}
			if !ps6136ProjectorHeadWidth(context, phi.Edges[index], projectors, model, modelType, head, shape, axis, budget-1) {
				return false
			}
			found = true
		}
		return found
	}
	named, ok := types.Unalias(value.Type()).(*types.Named)
	if !ok || projectors[named] == nil {
		return false
	}
	structure, ok := named.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for index := 0; index < structure.NumFields(); index++ {
		if structure.Field(index) != projectors[named] {
			continue
		}
		width := context.structField(value, index)
		return width.value != nil && ps6136HeadWidth(width.context, width.value, model, modelType, head, shape, axis, budget-1)
	}
	return false
}

// ps6136HeadWidth proves only source flow from the selected model's exact head
// tensor through Shape()[axis] to a projector width. Equality with cfg.Vocab,
// valid tensor shape and native projector semantics require separate reviewed
// facts. Invariance and complete owner observations are independent gates.
func ps6136HeadWidth(context *ps6125SSAContext, value ssa.Value, model ps6125SSAReference, modelType *types.Named, head *types.Var, shape string, axis int, budget int) bool {
	if context == nil || value == nil || model.value == nil || modelType == nil || head == nil || axis < 0 || budget <= 0 {
		return false
	}
	reference := context.reference(value)
	if reference.value == nil {
		return false
	}
	if reference.context != context || reference.value != value {
		return ps6136HeadWidth(reference.context, reference.value, model, modelType, head, shape, axis, budget-1)
	}
	if call, ok := value.(*ssa.Call); ok {
		child := ps6136Call(context, call)
		if child == nil {
			return false
		}
		found := false
		for _, block := range child.flow.function.Blocks {
			if !child.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				ret, ok := instruction.(*ssa.Return)
				if !ok {
					continue
				}
				if len(ret.Results) != 1 || !ps6136HeadWidth(child, ret.Results[0], model, modelType, head, shape, axis, budget-1) {
					return false
				}
				found = true
			}
		}
		return found
	}
	if phi, ok := value.(*ssa.Phi); ok {
		found := false
		for index, predecessor := range phi.Block().Preds {
			if !context.flow.edges[ps6125SSAEdge{from: predecessor, to: phi.Block()}] {
				continue
			}
			if !ps6136HeadWidth(context, phi.Edges[index], model, modelType, head, shape, axis, budget-1) {
				return false
			}
			found = true
		}
		return found
	}
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL || !types.Identical(value.Type(), types.Typ[types.Int]) {
		return false
	}
	index, ok := load.X.(*ssa.IndexAddr)
	if !ok {
		return false
	}
	integer := context.scalar(index.Index)
	if integer.state != ps6125Integer || !ps6125SameExtent(integer.extent, ps6125ConstantExtent(int64(axis))) {
		return false
	}
	base := context.reference(index.X)
	call, ok := base.value.(*ssa.Call)
	if !ok || call.Call.StaticCallee() == nil || len(call.Call.Args) != 1 {
		return false
	}
	function, ok := call.Call.StaticCallee().Object().(*types.Func)
	if !ok || ps6090FunctionID(function) != shape {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() == nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 || signature.Params().Len() != 0 || signature.Results().Len() != 1 {
		return false
	}
	slice, ok := call.Type().Underlying().(*types.Slice)
	if !ok || !types.Identical(slice.Elem(), types.Typ[types.Int]) {
		return false
	}
	receiver := base.context.reference(call.Call.Args[0])
	if receiver.value == nil {
		return false
	}
	paths := ps6125AccessPaths{flow: receiver.context.flow}
	path := paths.resolve(receiver.value)
	return path.known && len(path.access.fields) == 1 && path.access.fields[0] == head && ps6136AccessOwnerRoot(receiver.context, path, modelType) == model
}
