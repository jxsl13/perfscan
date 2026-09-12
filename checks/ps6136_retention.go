package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136FactoryRetention proves the successful result is appended exactly once
// to this owner's retention list and stored only in its returned fresh slot.
// It does not infer native exclusivity, successful execution or final release.
func ps6136FactoryRetention(context *ps6125SSAContext, backend *ssa.Call, owner ps6125SSAReference, ownerType *types.Named, list, slot *types.Var) bool {
	if context == nil || backend == nil || owner.value == nil || ownerType == nil || list == nil || slot == nil {
		return false
	}
	paths := ps6125AccessPaths{flow: context.flow}
	var result *ssa.Extract
	users := backend.Referrers()
	if users == nil {
		return false
	}
	for _, user := range *users {
		if extract, ok := user.(*ssa.Extract); ok && extract.Index == 0 {
			if result != nil {
				return false
			}
			result = extract
		}
	}
	if result == nil {
		return false
	}
	var appendCall *ssa.Call
	var array *ssa.Alloc
	var returnedSlot *ssa.Alloc
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			if returned, ok := instruction.(*ssa.Return); ok {
				allocation, fields, known := context.returnedFields(returned, 0)
				if !known {
					return false
				}
				if fields[slot].value == result {
					candidate, ok := allocation.value.(*ssa.Alloc)
					if !ok || allocation.context != context || returnedSlot != nil {
						return false
					}
					returnedSlot = candidate
				}
			}
			store, ok := instruction.(*ssa.Store)
			if !ok {
				continue
			}
			path := paths.resolve(store.Addr)
			if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != list {
				continue
			}
			if ps6136AccessOwnerRoot(context, path, ownerType) != owner || appendCall != nil {
				return false
			}
			call, ok := store.Val.(*ssa.Call)
			if !ok {
				return false
			}
			builtin, ok := call.Call.Value.(*ssa.Builtin)
			if !ok || builtin.Name() != "append" || len(call.Call.Args) != 2 {
				return false
			}
			current := paths.resolve(call.Call.Args[0])
			if !current.known || len(current.access.fields) != 1 || current.access.fields[0] != list || ps6136AccessOwnerRoot(context, current, ownerType) != owner {
				return false
			}
			candidate := ps6136SingleAppend(context, call.Call.Args[1], call, result)
			if candidate == nil {
				return false
			}
			appendCall, array = call, candidate
		}
	}
	if appendCall == nil || returnedSlot == nil {
		return false
	}
	resultUsers := result.Referrers()
	if resultUsers == nil {
		return false
	}
	for _, user := range *resultUsers {
		if user.Parent() == context.flow.function && !context.flow.blocks[user.Block()] {
			continue
		}
		switch use := user.(type) {
		case *ssa.DebugRef:
		case *ssa.Store:
			switch address := use.Addr.(type) {
			case *ssa.IndexAddr:
				if address.X != array {
					return false
				}
			case *ssa.FieldAddr:
				path := paths.resolve(address)
				if address.X != returnedSlot || !path.known || len(path.access.fields) != 1 || path.access.fields[0] != slot {
					return false
				}
			default:
				return false
			}
		default:
			return false // return aliases, extra helpers, captures, stores and conversions
		}
	}
	return true
}

func ps6136SingleAppend(context *ps6125SSAContext, value ssa.Value, call *ssa.Call, result ssa.Value) *ssa.Alloc {
	slice, ok := value.(*ssa.Slice)
	if !ok || slice.Low != nil || slice.High != nil || slice.Max != nil {
		return nil
	}
	array, ok := slice.X.(*ssa.Alloc)
	if !ok || array.Parent() != context.flow.function {
		return nil
	}
	pointer, ok := array.Type().(*types.Pointer)
	if !ok {
		return nil
	}
	shape, ok := pointer.Elem().Underlying().(*types.Array)
	if !ok || shape.Len() != 1 || !types.Identical(shape.Elem(), result.Type()) {
		return nil
	}
	users := array.Referrers()
	if users == nil {
		return nil
	}
	stores, slices := 0, 0
	for _, user := range *users {
		switch use := user.(type) {
		case *ssa.DebugRef:
		case *ssa.Slice:
			if use != slice {
				return nil
			}
			slices++
		case *ssa.IndexAddr:
			index := context.scalar(use.Index)
			if index.state != ps6125Integer || !ps6125SameExtent(index.extent, ps6125ConstantExtent(0)) {
				return nil
			}
			addressUsers := use.Referrers()
			if addressUsers == nil {
				return nil
			}
			for _, addressUser := range *addressUsers {
				if _, debug := addressUser.(*ssa.DebugRef); debug {
					continue
				}
				store, ok := addressUser.(*ssa.Store)
				if !ok || store.Addr != use || store.Val != result || !context.flow.instructionDominates(store, call) {
					return nil
				}
				stores++
			}
		default:
			return nil
		}
	}
	if stores != 1 || slices != 1 {
		return nil
	}
	sliceUsers := slice.Referrers()
	if sliceUsers == nil {
		return nil
	}
	for _, user := range *sliceUsers {
		if _, debug := user.(*ssa.DebugRef); !debug && user != call {
			return nil
		}
	}
	return array
}
