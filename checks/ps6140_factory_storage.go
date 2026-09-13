package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type ps6140FactoryStorageProof struct {
	factory, callback *ps6125SSAContext
	backend, native   *ssa.Call
	wrapper           *types.Named
	nativeField       *types.Var
}

// Join source descriptor/result identity with exhaustive host-input and boxed
// wrapper uses. Existing factory retention closes the backend result's stores;
// constructor storage closes the returned slot. The named native leaf is a
// reviewed boundary, not evidence of native copying, disjointness or lifetime.
func ps6140FactoryStorage(factory *ps6125SSAContext, allocator, slot *types.Var, identities map[string]bool, budget int) *ps6140FactoryStorageProof {
	if factory == nil || factory.flow == nil || len(factory.flow.function.Params) != 1 || allocator == nil || slot == nil || budget <= 0 {
		return nil
	}
	parameter := factory.flow.function.Params[0]
	input := factory.reference(parameter)
	backend := ps6136FactoryResult(factory, input, allocator, slot)
	if backend == nil || !ps6136BoundAllocator(factory, backend, input, identities) || !ps6140OnlyConstructorUse(factory, parameter, backend, &budget) {
		return nil
	}
	callback := factory.call(backend)
	if callback == nil || len(callback.flow.function.Params) != 1 || callback.reference(callback.flow.function.Params[0]) != input {
		return nil
	}
	proof := &ps6140FactoryStorageProof{factory: factory, callback: callback, backend: backend}
	for _, block := range callback.flow.function.Blocks {
		if !callback.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			budget--
			if budget <= 0 {
				return nil
			}
			if call, ok := instruction.(*ssa.Call); ok {
				callee := call.Call.StaticCallee()
				if callee != nil {
					object, ok := callee.Object().(*types.Func)
					if ok && identities[ps6090FunctionID(object)] {
						if proof.native != nil {
							return nil
						}
						proof.native = call
					}
				}
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok || len(returned.Results) != 2 {
				continue
			}
			if constant, ok := returned.Results[0].(*ssa.Const); ok && constant.IsNil() {
				continue
			}
			boxed, ok := returned.Results[0].(*ssa.MakeInterface)
			if !ok || !ps6140OnlyConstructorUse(callback, boxed, returned, &budget) {
				return nil
			}
			wrapper, ok := boxed.X.Type().(*types.Named)
			if !ok || proof.wrapper != nil {
				return nil
			}
			structure, ok := wrapper.Underlying().(*types.Struct)
			if !ok || structure.NumFields() != 1 || !ps6136BoundBufferWrapper(factory, backend, wrapper, structure.Field(0)) {
				return nil
			}
			load, ok := boxed.X.(*ssa.UnOp)
			if !ok || load.Op != token.MUL || !ps6140OnlyConstructorUse(callback, load, boxed, &budget) {
				return nil
			}
			cell, ok := load.X.(*ssa.Alloc)
			if !ok || callback.structCell(cell) == nil || cell.Referrers() == nil {
				return nil
			}
			// structCell proves initialization, not exclusive use of subsequent
			// loaded copies. Require this sole read of the closed wrapper cell.
			for _, user := range *cell.Referrers() {
				budget--
				if budget <= 0 || user.Parent() != callback.flow.function {
					return nil
				}
				if !callback.flow.blocks[user.Block()] {
					continue
				}
				switch use := user.(type) {
				case *ssa.DebugRef:
				case *ssa.UnOp:
					if use != load {
						return nil
					}
				case *ssa.FieldAddr:
					if use.Referrers() == nil {
						return nil
					}
					for _, access := range *use.Referrers() {
						budget--
						if budget <= 0 {
							return nil
						}
						if _, debug := access.(*ssa.DebugRef); debug {
							continue
						}
						store, ok := access.(*ssa.Store)
						if !ok || store.Addr != use {
							return nil
						}
					}
				default:
					return nil
				}
			}
			proof.wrapper, proof.nativeField = wrapper, structure.Field(0)
		}
	}
	if proof.native == nil || proof.wrapper == nil || !ps6140OnlyConstructorUse(callback, callback.flow.function.Params[0], proof.native, &budget) {
		return nil
	}
	return proof
}
