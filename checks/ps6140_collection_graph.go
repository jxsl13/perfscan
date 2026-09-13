package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Collection/element metadata only. Native handles loaded from a block remain
// separate effect/alias/lifetime obligations, not read-only native operations.
type ps6140CollectionGraph struct {
	block      *types.Named
	ownerType  *types.Named
	blocks     *types.Var
	projectors map[*types.Var]*types.Named
	remaining  int
	seen       map[ps6125SSAReference]bool
	last       string
}

func (graph *ps6140CollectionGraph) take() bool {
	if graph.remaining <= 0 {
		return false
	}
	graph.remaining--
	return true
}

func (graph *ps6140CollectionGraph) readOnly(context *ps6125SSAContext, value ssa.Value) bool {
	if context == nil || context.flow == nil || value == nil || !graph.take() {
		return false
	}
	key := ps6125SSAReference{context: context, value: value}
	if graph.seen[key] {
		return true
	}
	graph.seen[key] = true
	users := value.Referrers()
	if users == nil {
		return false
	}
	for _, instruction := range *users {
		graph.last = instruction.String()
		if !graph.take() || instruction.Parent() != context.flow.function {
			return false
		}
		if !context.flow.blocks[instruction.Block()] {
			continue
		}
		switch user := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.FieldAddr:
			if user.X != value || !graph.readOnly(context, user) {
				return false
			}
		case *ssa.IndexAddr:
			if user.X != value || !types.Identical(user.Type(), types.NewPointer(graph.block)) || !graph.readOnly(context, user) {
				return false
			}
		case *ssa.UnOp:
			if user.Op != token.MUL || user.X != value {
				return false
			}
			if types.Identical(user.Type(), graph.block) || types.Identical(user.Type(), types.NewPointer(graph.block)) {
				if !graph.readOnly(context, user) {
					return false
				}
			} else if field, ok := value.(*ssa.FieldAddr); ok {
				pointer, ok := field.X.Type().Underlying().(*types.Pointer)
				if !ok {
					return false
				}
				structure, ok := pointer.Elem().Underlying().(*types.Struct)
				if !ok || field.Field < 0 || field.Field >= structure.NumFields() {
					return false
				}
				if concrete := graph.projectors[structure.Field(field.Field)]; concrete != nil && !graph.projectorValue(context, user, concrete) {
					return false
				}
			}
		case *ssa.Field:
			if user.X != value || !types.Identical(value.Type(), graph.block) {
				return false
			}
			structure := graph.block.Underlying().(*types.Struct)
			if user.Field < 0 || user.Field >= structure.NumFields() {
				return false
			}
			if concrete := graph.projectors[structure.Field(user.Field)]; concrete != nil && !graph.projectorValue(context, user, concrete) {
				return false
			}
		case *ssa.Store:
			if allocation, ok := value.(*ssa.Alloc); ok && user.Addr == allocation && graph.seen[ps6125SSAReference{context: context, value: user.Val}] {
				if ps6140WholeBlockCopyStore(context, allocation) == user {
					continue
				}
				return false
			}
			// Whole block copies must be private, unique closed cells. No
			// element/field store or global/container/captured copy is allowed.
			allocation, ok := user.Addr.(*ssa.Alloc)
			if !ok || user.Val != value || !types.Identical(allocation.Type(), types.NewPointer(graph.block)) {
				return false
			}
			if ps6140WholeBlockCopyStore(context, allocation) != user || !graph.readOnly(context, allocation) {
				return false
			}
		case *ssa.Call:
			if builtin, ok := user.Call.Value.(*ssa.Builtin); ok {
				if (builtin.Name() == "len" || builtin.Name() == "cap") && len(user.Call.Args) == 1 && user.Call.Args[0] == value {
					continue
				}
				if builtin.Name() == "append" && len(user.Call.Args) == 2 && user.Call.Args[0] == value {
					if !graph.freshAppend(context, user) {
						return false
					}
					continue
				}
				return false
			}
			if user.Call.IsInvoke() {
				return false
			}
			child := ps6136Call(context, user)
			if child == nil || len(child.flow.function.Blocks) == 0 || len(child.flow.function.Params) != len(user.Call.Args) {
				return false
			}
			found := false
			for index, argument := range user.Call.Args {
				if argument == value {
					found = true
					if !graph.readOnly(child, child.flow.function.Params[index]) {
						return false
					}
				}
			}
			if !found {
				return false
			}
		default:
			return false // slices, phis, returns, captures, sends, go/defer, aliases
		}
	}
	return true
}

func ps6140WholeBlockCopyStore(context *ps6125SSAContext, allocation *ssa.Alloc) *ssa.Store {
	if allocation.Referrers() == nil || allocation.Parent() != context.flow.function {
		return nil
	}
	var stored *ssa.Store
	for _, instruction := range *allocation.Referrers() {
		if !context.flow.blocks[instruction.Block()] {
			continue
		}
		if store, ok := instruction.(*ssa.Store); ok {
			if store.Addr != allocation || stored != nil {
				return nil
			}
			stored = store
		}
	}
	if stored == nil {
		return nil
	}
	for _, instruction := range *allocation.Referrers() {
		if instruction == stored || !context.flow.blocks[instruction.Block()] {
			continue
		}
		if _, debug := instruction.(*ssa.DebugRef); debug {
			continue
		}
		if !context.flow.instructionDominates(stored, instruction) {
			return nil
		}
	}
	return stored
}

func (graph *ps6140CollectionGraph) projectorValue(context *ps6125SSAContext, value ssa.Value, concrete *types.Named) bool {
	users := value.Referrers()
	if users == nil {
		return false
	}
	iface, ok := value.Type().Underlying().(*types.Interface)
	if !ok || !types.Implements(concrete, iface) {
		return false
	}
	for _, instruction := range *users {
		graph.last = instruction.String()
		if !graph.take() || instruction.Parent() != context.flow.function {
			return false
		}
		if !context.flow.blocks[instruction.Block()] {
			continue
		}
		if _, debug := instruction.(*ssa.DebugRef); debug {
			continue
		}
		call, ok := instruction.(*ssa.Call)
		if !ok || !call.Call.IsInvoke() || call.Call.Value != value || call.Call.Method == nil {
			return false
		}
		object, _, _ := types.LookupFieldOrMethod(concrete, false, call.Call.Method.Pkg(), call.Call.Method.Name())
		method, ok := object.(*types.Func)
		if !ok || method.Id() != call.Call.Method.Id() {
			return false
		}
		signature := method.Type().(*types.Signature)
		function := context.flow.function.Prog.FuncValue(method)
		if signature.Recv() == nil || !types.Identical(signature.Recv().Type(), concrete) || function == nil || len(function.Blocks) == 0 {
			return false
		}
		// A Go value receiver cannot replace the original interface slot.
		// All source/native effects of this method remain separate obligations.
	}
	return true
}

func (graph *ps6140CollectionGraph) freshAppend(context *ps6125SSAContext, call *ssa.Call) bool {
	if ps6140AppendedBlock(context, call, graph.block).value == nil {
		graph.last += " [descriptor]"
		return false
	}
	users := call.Referrers()
	if users == nil || len(*users) != 1 {
		graph.last += " [users]"
		return false
	}
	store, ok := (*users)[0].(*ssa.Store)
	if !ok || store.Val != call {
		return false
	}
	paths := ps6125AccessPaths{flow: context.flow}
	path := paths.resolve(store.Addr)
	if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != graph.blocks {
		field, exact := store.Addr.(*ssa.FieldAddr)
		load, loaded := call.Call.Args[0].(*ssa.UnOp)
		if !exact || !loaded || load.Op != token.MUL || !types.Identical(field.X.Type(), types.NewPointer(graph.ownerType)) {
			graph.last += " [destination]"
			return false
		}
		structure := graph.ownerType.Underlying().(*types.Struct)
		if field.Field < 0 || field.Field >= structure.NumFields() || structure.Field(field.Field) != graph.blocks {
			return false
		}
		inputField, exact := load.X.(*ssa.FieldAddr)
		if !exact || inputField.Field != field.Field || !types.Identical(inputField.X.Type(), field.X.Type()) {
			return false
		}
		root := ps6136OwnerRoot(context, field.X, graph.ownerType)
		_, fresh := root.value.(*ssa.Alloc)
		return fresh && ps6136OwnerRoot(context, inputField.X, graph.ownerType) == root
	}
	owner := ps6136AccessOwnerRoot(context, path, graph.ownerType)
	_, fresh := owner.value.(*ssa.Alloc)
	input := paths.resolve(call.Call.Args[0])
	if !fresh {
		graph.last += " [owner not alloc]"
	}
	if !input.known {
		graph.last += " [input unknown]"
	}
	return fresh && input.known && len(input.access.fields) == 1 && input.access.fields[0] == graph.blocks && ps6136AccessOwnerRoot(context, input, graph.ownerType) == owner
}
