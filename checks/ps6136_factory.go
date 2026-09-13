package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136FactoryResult verifies actual input and returned-slot provenance, not
// successful allocation or native ownership. The caller must additionally
// prove same-owner retention, typed backend binding, error cleanup and release.
func ps6136FactoryResult(context *ps6125SSAContext, input ps6125SSAReference, allocator, slot *types.Var) *ssa.Call {
	if context == nil || input.value == nil || allocator == nil || slot == nil {
		return nil
	}
	signature, ok := allocator.Type().Underlying().(*types.Signature)
	if !ok || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.Params().Len() != 1 || signature.Results().Len() != 2 ||
		!types.Identical(signature.Params().At(0).Type(), types.NewSlice(types.Typ[types.Float32])) ||
		!types.Identical(signature.Results().At(0).Type(), slot.Type()) ||
		!types.Identical(signature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil
	}
	paths := ps6125AccessPaths{flow: context.flow}
	var backend *ssa.Call
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			path := paths.resolve(call.Call.Value)
			if !path.known || len(path.access.fields) == 0 || path.access.fields[len(path.access.fields)-1] != allocator {
				continue
			}
			if backend != nil || call.Call.IsInvoke() || len(call.Call.Args) != 1 || context.reference(call.Call.Args[0]) != input {
				return nil
			}
			backend = call
		}
	}
	if backend == nil {
		return nil
	}
	populated, empty := 0, 0
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			if len(returned.Results) != 1 {
				return nil
			}
			_, fields, known := context.returnedFields(returned, 0)
			if !known {
				return nil
			}
			reference := fields[slot]
			if reference.value == nil {
				empty++
				continue
			}
			extracted, ok := reference.value.(*ssa.Extract)
			if !ok || reference.context != context || extracted.Tuple != backend || extracted.Index != 0 {
				return nil
			}
			populated++
		}
	}
	if populated != 1 || empty != 2 {
		return nil
	}
	return backend
}
