package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6140ConstructorAllocation reuses source factory/error/ownership seams but
// does not reuse output-head/common-row semantics. The caller must separately
// close selected block dispatch, all field/owner observations and native
// ownership facts before interpreting this as unused retained residency.
func ps6140ConstructorAllocation(selection *ps6136Selection, remaining int) *ps6136ConstructorProof {
	if selection == nil || !selection.initializationValues(remaining) || selection.workspace == nil || selection.allocator == nil || selection.ops == nil || selection.slot == nil || selection.retained == nil {
		return nil
	}
	model, ok := selection.model.value.(*ssa.Parameter)
	modelObject := ps6136ParameterObject(model)
	if !ok || modelObject == nil {
		return nil
	}
	var pool ps6125ExtentPool
	rows := pool.identity(modelObject, []*types.Var{selection.modelConfig, selection.configRows}, false)
	width := pool.identity(modelObject, []*types.Var{selection.modelConfig, selection.configWidth}, false)
	// Only retained=4*rows*width is relevant here. PS6136's additional common/
	// idle arithmetic is NOT the unused-scratch candidate's memory model.
	residency, ok := ps6136OutputResidency(rows, width)
	if !ok {
		return nil
	}
	facts := &ps6136Extents{owner: selection.owner, ownerType: selection.ownerType, immutable: map[*types.Var]ps6125Extent{selection.maximumRows: ps6125SymbolicExtent(rows), selection.width: ps6125SymbolicExtent(width)}, remaining: remaining}
	proof := &ps6136ConstructorProof{residency: residency}
	pending := []*ps6125SSAContext{selection.context}
	seen := make(map[*ps6125SSAContext]bool)
	for len(pending) > 0 {
		context := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[context] {
			continue
		}
		seen[context] = true
		remaining--
		if remaining <= 0 {
			return nil
		}
		paths := ps6125AccessPaths{flow: context.flow}
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				if call, ok := instruction.(*ssa.Call); ok {
					if child := ps6136Call(context, call); child != nil {
						pending = append(pending, child)
					}
				}
				store, ok := instruction.(*ssa.Store)
				if !ok {
					continue
				}
				path := paths.resolve(store.Addr)
				if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != selection.workspace || ps6136AccessOwnerRoot(context, path, selection.ownerType) != selection.owner {
					continue
				}
				function, ok := context.flow.function.Object().(*types.Func)
				if proof.allocation != nil || !ok || ps6090FunctionID(function) != selection.allocationFunction {
					return nil
				}
				call, ok := store.Val.(*ssa.Call)
				if !ok || len(call.Call.Args) != 1 {
					return nil
				}
				input := context.reference(call.Call.Args[0])
				allocation, ok := input.value.(*ssa.MakeSlice)
				full := ps6125MultiplyExtents(ps6125SymbolicExtent(rows), ps6125SymbolicExtent(width))
				if !ok || !types.Identical(allocation.Type(), types.NewSlice(types.Typ[types.Float32])) || !ps6125SameExtent(facts.length(input.context, allocation), full) || !ps6125SameExtent(facts.extent(input.context, allocation.Cap), full) {
					return nil
				}
				factory := ps6136Call(context, call)
				backend := ps6136FactoryResult(factory, input, selection.allocator, selection.slot)
				if backend == nil || !ps6136FactoryRetention(factory, backend, selection.owner, selection.ownerType, selection.retained, selection.slot) {
					return nil
				}
				if factory.call(backend) == nil {
					snapshot := ps6136InitialOwnerField(selection.owner, selection.ops)
					if snapshot.value == nil {
						return nil
					}
					origin := snapshot.context.structField(snapshot.value, selection.allocatorIndex)
					if ps6136CallOrigin(factory, backend, origin) == nil {
						return nil
					}
				}
				if !ps6136BoundAllocator(factory, backend, input, selection.allocatorIDs) {
					return nil
				}
				errorCell := ps6136FactoryErrorCell(factory, backend)
				if errorCell.value == nil || !ps6136ConstructorErrorBarrier(selection.context, selection.owner, selection.ownerType, errorCell, selection.releaseMethod) {
					return nil
				}
				proof.allocation, proof.factory, proof.errorCell = store, factory, errorCell
			}
		}
	}
	if proof.allocation == nil {
		return nil
	}
	return proof
}
