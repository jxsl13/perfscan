package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// returnedFields proves explicit field values at one return of a fresh local
// struct allocation. It closes uses of the struct's address: unknown calls,
// captures, address exports, conversions and whole-struct stores reject it.
// Missing fields are unknown, not assumed zero. This says nothing about the
// lifetime, contents or capacity of objects referenced by the stored values.
func (origins *ps6125SSAOrigins) returnedFields(returned *ssa.Return, index int) (*ssa.Alloc, map[*types.Var]ssa.Value, bool) {
	if origins.flow == nil || returned == nil || returned.Parent() != origins.flow.function || !origins.flow.blocks[returned.Block()] || index < 0 || index >= len(returned.Results) {
		return nil, nil, false
	}
	resolved := origins.resolve(returned.Results[index])
	allocation, ok := resolved.value.(*ssa.Alloc)
	if !resolved.known || !ok || allocation.Parent() != origins.flow.function {
		return nil, nil, false
	}
	pointer, ok := types.Unalias(allocation.Type()).Underlying().(*types.Pointer)
	if !ok {
		return nil, nil, false
	}
	structure, ok := types.Unalias(pointer.Elem()).Underlying().(*types.Struct)
	if !ok {
		return nil, nil, false
	}
	stores := make(map[*types.Var][]*ssa.Store)
	pending := []ssa.Value{allocation}
	seen := make(map[ssa.Value]bool)
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
			return nil, nil, false
		}
		for _, user := range *users {
			if user.Parent() != origins.flow.function {
				return nil, nil, false
			}
			if !origins.flow.blocks[user.Block()] {
				continue
			}
			switch use := user.(type) {
			case *ssa.DebugRef, *ssa.Return:
				// Returning the struct does not expose its address until this
				// invocation has ended; all writes below remain source-local.
			case *ssa.Phi:
				joined := origins.resolve(use)
				if !joined.known || joined.value != allocation {
					return nil, nil, false
				}
				pending = append(pending, use)
			case *ssa.FieldAddr:
				if use.X != address || use.Field < 0 || use.Field >= structure.NumFields() {
					return nil, nil, false
				}
				field := structure.Field(use.Field)
				fieldUsers := use.Referrers()
				if fieldUsers == nil {
					return nil, nil, false
				}
				for _, fieldUser := range *fieldUsers {
					if fieldUser.Parent() != origins.flow.function {
						return nil, nil, false
					}
					if !origins.flow.blocks[fieldUser.Block()] {
						continue
					}
					switch operation := fieldUser.(type) {
					case *ssa.Store:
						if operation.Addr != use {
							return nil, nil, false
						}
						stores[field] = append(stores[field], operation)
					case *ssa.UnOp:
						if operation.Op != token.MUL || operation.X != use {
							return nil, nil, false
						}
					case *ssa.DebugRef:
					default:
						return nil, nil, false
					}
				}
			default:
				return nil, nil, false
			}
		}
	}
	values := make(map[*types.Var]ssa.Value, len(stores))
	for field, writes := range stores {
		if len(writes) == 1 && origins.flow.instructionDominates(writes[0], returned) {
			values[field] = writes[0].Val
		}
	}
	return allocation, values, true
}

func (flow *ps6125SSAExtents) instructionDominates(before, after ssa.Instruction) bool {
	if before.Parent() != flow.function || after.Parent() != flow.function || !flow.blocks[before.Block()] || !flow.blocks[after.Block()] {
		return false
	}
	if before.Block() != after.Block() {
		if before.Block().Dominates(after.Block()) {
			return true
		}
		// Excluding the candidate must disconnect the target from entry on
		// every executable path. Structural SSA dominators alone retain
		// branches already excluded by this invocation's parameter facts.
		pending := []*ssa.BasicBlock{flow.function.Blocks[0]}
		seen := make(map[*ssa.BasicBlock]bool)
		for len(pending) != 0 {
			last := len(pending) - 1
			block := pending[last]
			pending = pending[:last]
			if block == before.Block() || seen[block] {
				continue
			}
			if block == after.Block() {
				return false
			}
			seen[block] = true
			for _, successor := range block.Succs {
				if flow.edges[ps6125SSAEdge{from: block, to: successor}] {
					pending = append(pending, successor)
				}
			}
		}
		return true
	}
	for _, instruction := range before.Block().Instrs {
		if instruction == before {
			return true
		}
		if instruction == after {
			return false
		}
	}
	return false
}
