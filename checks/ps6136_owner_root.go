package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136OwnerRoot resolves an exact owner pointer through a closed captured
// pointer cell. Unlike PS6125 callable cells, this is an identity-only proof:
// it establishes neither immutable owner memory nor device-resource lifetime.
func ps6136OwnerRoot(context *ps6125SSAContext, value ssa.Value, owner *types.Named) ps6125SSAReference {
	if context == nil || value == nil || owner == nil || !types.Identical(value.Type(), types.NewPointer(owner)) {
		return ps6125SSAReference{}
	}
	reference := context.reference(value)
	seen := make(map[ps6125SSAReference]bool)
	for reference.value != nil && !seen[reference] {
		seen[reference] = true
		call, isCall := reference.value.(*ssa.Call)
		resultIndex := 0
		if extraction, ok := reference.value.(*ssa.Extract); ok {
			call, isCall = extraction.Tuple.(*ssa.Call)
			resultIndex = extraction.Index
		}
		if isCall {
			child := ps6136Call(reference.context, call)
			if child == nil {
				break
			}
			var returnedRoot ps6125SSAReference
			valid := true
			for _, block := range child.flow.function.Blocks {
				if !child.flow.blocks[block] {
					continue
				}
				for _, instruction := range block.Instrs {
					returned, ok := instruction.(*ssa.Return)
					if !ok {
						continue
					}
					if resultIndex >= len(returned.Results) {
						valid = false
						continue
					}
					value := returned.Results[resultIndex]
					// Nil is not a competing owner identity. The caller must still
					// establish successful construction before resource use.
					if constant, ok := value.(*ssa.Const); ok && constant.IsNil() {
						continue
					}
					root := ps6136OwnerRoot(child, value, owner)
					_, fresh := root.value.(*ssa.Alloc)
					if !fresh || returnedRoot.value != nil && returnedRoot != root {
						valid = false
					}
					returnedRoot = root
				}
			}
			if !valid || returnedRoot.value == nil {
				break
			}
			reference = returnedRoot
			continue
		}
		switch reference.value.(type) {
		case *ssa.Parameter, *ssa.Alloc:
			if types.Identical(reference.value.Type(), types.NewPointer(owner)) {
				return reference
			}
		}
		load, ok := reference.value.(*ssa.UnOp)
		if !ok || load.Op != token.MUL {
			break
		}
		cell := reference.context.reference(load.X)
		allocation, ok := cell.value.(*ssa.Alloc)
		if !ok || !types.Identical(allocation.Type(), types.NewPointer(types.NewPointer(owner))) {
			break
		}
		store := ps6136ClosedOwnerCell(cell.context, allocation)
		if store == nil {
			break
		}
		var point ssa.Instruction = load
		current := reference.context
		for current != cell.context && current.parent != nil {
			point = current.site
			current = current.parent
		}
		if current != cell.context {
			// A returned closure's creating function is not on the invocation
			// stack. Require this exact closed cell among its source captures
			// and initialization before creation (stronger than before return).
			point = ps6136ReturnedCapturePoint(reference.context, cell)
		}
		if point == nil || !cell.context.flow.instructionDominates(store, point) {
			break
		}
		reference = cell.context.reference(store.Val)
	}
	return ps6125SSAReference{}
}

func ps6136ReturnedCapturePoint(context *ps6125SSAContext, cell ps6125SSAReference) ssa.Instruction {
	for current := context; current != nil && current.parent != nil; current = current.parent {
		if current.site == nil {
			continue
		}
		origin := ps6136ReturnedCallable(current.parent.reference(current.site.Call.Value), make(map[ps6125SSAReference]bool))
		closure, ok := origin.value.(*ssa.MakeClosure)
		if !ok || origin.context != cell.context {
			continue
		}
		for _, capture := range closure.Bindings {
			if origin.context.reference(capture) == cell {
				return closure
			}
		}
	}
	return nil
}

func ps6136ClosedOwnerCell(context *ps6125SSAContext, allocation *ssa.Alloc) *ssa.Store {
	if context == nil || allocation.Parent() != context.flow.function || !context.flow.blocks[allocation.Block()] {
		return nil
	}
	pending := []ssa.Value{allocation}
	seen := make(map[ssa.Value]bool)
	var store *ssa.Store
	for len(pending) > 0 {
		address := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
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
	return store
}

func ps6136AccessOwnerRoot(context *ps6125SSAContext, path ps6125AccessResult, owner *types.Named) ps6125SSAReference {
	if !path.known {
		return ps6125SSAReference{}
	}
	if root := ps6136OwnerRoot(context, path.access.root, owner); root.value != nil {
		return root
	}
	for _, load := range path.access.loads {
		if root := ps6136OwnerRoot(context, load, owner); root.value != nil {
			return root
		}
	}
	return ps6125SSAReference{}
}
