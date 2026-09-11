package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// callable additionally permits a function-value load from a closed local cell.
// The cell must have one source store, dominate the relevant invocation point,
// and never be written by a capturing closure or exported as an address. This
// is a callable-source proof, not a general memory-load or instance-identity rule.
func (context *ps6125SSAContext) callable(value ssa.Value) ps6125SSAReference {
	reference := context.reference(value)
	seen := make(map[ps6125SSAReference]bool)
	for reference.value != nil && !seen[reference] {
		seen[reference] = true
		if function, _ := ps6125SSACallee(reference.value); function != nil {
			return reference
		}
		load, ok := reference.value.(*ssa.UnOp)
		if !ok || load.Op != token.MUL {
			break
		}
		cell := reference.context.reference(load.X)
		allocation, ok := cell.value.(*ssa.Alloc)
		if !ok {
			break
		}
		store := cell.context.functionCell(allocation)
		if store == nil {
			break
		}
		// For a nested callback, initialization must precede the call edge
		// leaving the cell's context. Closure creation alone is insufficient:
		// a closure can be created before initialization and invoked afterward.
		var point ssa.Instruction = load
		current := reference.context
		for current != cell.context && current.parent != nil {
			point = current.site
			current = current.parent
		}
		if current != cell.context || !cell.context.flow.instructionDominates(store, point) {
			break
		}
		reference = cell.context.reference(store.Val)
	}
	return ps6125SSAReference{}
}

func (context *ps6125SSAContext) functionCell(allocation *ssa.Alloc) *ssa.Store {
	if allocation.Parent() != context.flow.function || !context.flow.blocks[allocation.Block()] {
		return nil
	}
	pointer, ok := types.Unalias(allocation.Type()).Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	if _, ok := types.Unalias(pointer.Elem()).Underlying().(*types.Signature); !ok {
		return nil
	}
	if context.cells == nil {
		context.cells = make(map[*ssa.Alloc]*ssa.Store)
	}
	if result, queried := context.cells[allocation]; queried {
		return result
	}
	context.cells[allocation] = nil
	pending := []ssa.Value{allocation}
	seen := make(map[ssa.Value]bool)
	var store *ssa.Store
	for len(pending) != 0 {
		last := len(pending) - 1
		address := pending[last]
		pending = pending[:last]
		if seen[address] {
			continue
		}
		seen[address] = true
		users := address.Referrers()
		if users == nil {
			return nil
		}
		for _, user := range *users {
			if user.Parent() == context.flow.function && !context.flow.blocks[user.Block()] {
				continue
			}
			switch use := user.(type) {
			case *ssa.DebugRef:
			case *ssa.Store:
				if use.Addr != address || address != allocation || use.Parent() != context.flow.function || store != nil {
					return nil
				}
				store = use
			case *ssa.UnOp:
				if use.Op != token.MUL || use.X != address {
					return nil
				}
			case *ssa.MakeClosure:
				function, captures := ps6125SSACallee(use)
				if function == nil {
					return nil
				}
				for index, capture := range captures {
					if capture == address {
						pending = append(pending, function.FreeVars[index])
					}
				}
			default:
				return nil
			}
		}
	}
	context.cells[allocation] = store
	return store
}
