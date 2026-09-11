package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// A closed struct cell is either initialized once as a whole value or by
// separate single field stores. These are source facts, not heap invariants.
// Loads are projected only after checking initialization at the read's actual
// invocation edge. Missing fields, mixed writes and address escapes are unknown.
type ps6125SSAStructStores struct {
	whole  *ssa.Store
	fields []*ssa.Store
}

type ps6125SSAProjection struct {
	value ps6125SSAReference
	field int
}

func (context *ps6125SSAContext) structField(value ssa.Value, field int) ps6125SSAReference {
	return context.projectStruct(context.reference(value), field, make(map[ps6125SSAProjection]bool))
}

func (context *ps6125SSAContext) projectStruct(value ps6125SSAReference, field int, active map[ps6125SSAProjection]bool) ps6125SSAReference {
	if value.value == nil {
		return ps6125SSAReference{}
	}
	structure, ok := value.value.Type().Underlying().(*types.Struct)
	if !ok || field < 0 || field >= structure.NumFields() {
		return ps6125SSAReference{}
	}
	key := ps6125SSAProjection{value: value, field: field}
	if active[key] {
		return ps6125SSAReference{}
	}
	active[key] = true
	defer delete(active, key)
	switch source := value.value.(type) {
	case *ssa.Field:
		outer := context.projectStruct(value.context.reference(source.X), source.Field, active)
		return context.projectStruct(outer, field, active)
	case *ssa.UnOp:
		if source.Op == token.MUL {
			return value.context.structFieldAt(source.X, field, source, active)
		}
	}
	return ps6125SSAReference{}
}

func (context *ps6125SSAContext) loadedStructField(address ssa.Value, field int, load *ssa.UnOp) ps6125SSAReference {
	return context.structFieldAt(address, field, load, make(map[ps6125SSAProjection]bool))
}

func (context *ps6125SSAContext) structFieldAt(address ssa.Value, field int, load *ssa.UnOp, active map[ps6125SSAProjection]bool) ps6125SSAReference {
	if context.reference(load).value == nil {
		return ps6125SSAReference{}
	}
	cell := context.reference(address)
	allocation, ok := cell.value.(*ssa.Alloc)
	if !ok {
		return ps6125SSAReference{}
	}
	stores := cell.context.structCell(allocation)
	if stores == nil || field < 0 || field >= len(stores.fields) {
		return ps6125SSAReference{}
	}
	store := stores.whole
	if store == nil {
		store = stores.fields[field]
	}
	if store == nil {
		return ps6125SSAReference{}
	}
	var point ssa.Instruction = load
	current := context
	for current != cell.context && current.parent != nil {
		point = current.site
		current = current.parent
	}
	if current != cell.context || !cell.context.flow.instructionDominates(store, point) {
		return ps6125SSAReference{}
	}
	value := cell.context.reference(store.Val)
	if stores.whole != nil {
		return context.projectStruct(value, field, active)
	}
	return value
}

func (context *ps6125SSAContext) structCell(allocation *ssa.Alloc) *ps6125SSAStructStores {
	if allocation.Parent() != context.flow.function || !context.flow.blocks[allocation.Block()] {
		return nil
	}
	pointer, ok := allocation.Type().Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	structure, ok := pointer.Elem().Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	if context.structs == nil {
		context.structs = make(map[*ssa.Alloc]*ps6125SSAStructStores)
	}
	if stores, known := context.structs[allocation]; known {
		return stores
	}
	context.structs[allocation] = nil
	stores := &ps6125SSAStructStores{fields: make([]*ssa.Store, structure.NumFields())}
	fieldStored := false
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
			return nil
		}
		for _, user := range *users {
			if user.Parent() == context.flow.function && !context.flow.blocks[user.Block()] {
				continue
			}
			switch use := user.(type) {
			case *ssa.DebugRef:
			case *ssa.Store:
				if use.Addr != address || address != allocation || use.Parent() != context.flow.function || stores.whole != nil || fieldStored {
					return nil
				}
				stores.whole = use
			case *ssa.UnOp:
				if use.Op != token.MUL || use.X != address {
					return nil
				}
			case *ssa.FieldAddr:
				if use.X != address || use.Field < 0 || use.Field >= structure.NumFields() || use.Referrers() == nil {
					return nil
				}
				for _, fieldUser := range *use.Referrers() {
					if fieldUser.Parent() == context.flow.function && !context.flow.blocks[fieldUser.Block()] {
						continue
					}
					switch operation := fieldUser.(type) {
					case *ssa.DebugRef:
					case *ssa.UnOp:
						if operation.Op != token.MUL || operation.X != use {
							return nil
						}
					case *ssa.Store:
						if operation.Addr != use || operation.Parent() != context.flow.function || stores.whole != nil || stores.fields[use.Field] != nil {
							return nil
						}
						stores.fields[use.Field] = operation
						fieldStored = true
					default:
						return nil
					}
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
	context.structs[allocation] = stores
	return stores
}
