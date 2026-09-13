package checks

import (
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// A publication-time snapshot, not permission to specialize later owner reads.
// The caller must additionally close every constructor-time and later
// field/owner exposure, and join the same constructor lifetime before using
// these values at runtime. In particular, this local store census is not a
// package-wide alias/escape proof.
type ps6140ConstructorFlagSnapshot struct {
	constructor *ps6125SSAContext
	owner       ps6125SSAReference
	values      map[*types.Var]bool
}

type ps6140FlagStore struct {
	context *ps6125SSAContext
	store   *ssa.Store
	value   bool
}

func ps6140ConstructorFlags(constructor *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, fields map[*types.Var]bool, budget int) *ps6140ConstructorFlagSnapshot {
	return ps6140ConstructorFlagsCheck(constructor, owner, ownerType, fields, budget, nil)
}

func ps6140ConstructorFlagsCheck(constructor *ps6125SSAContext, owner ps6125SSAReference, ownerType *types.Named, fields map[*types.Var]bool, budget int, rejected func(string)) (result *ps6140ConstructorFlagSnapshot) {
	stage := "input/publication"
	defer func() {
		if result == nil && rejected != nil {
			rejected(stage)
		}
	}()
	if constructor == nil || constructor.flow == nil || constructor.flow.function == nil || ownerType == nil || len(fields) == 0 || budget <= 0 || owner.context == nil || owner.context.flow == nil {
		return nil
	}
	allocation, ok := owner.value.(*ssa.Alloc)
	if !ok || !types.Identical(allocation.Type(), types.NewPointer(ownerType)) {
		return nil
	}
	structure, ok := ownerType.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	isOwner := func(typ types.Type) bool {
		return types.Identical(typ, ownerType) || types.Identical(typ, types.NewPointer(ownerType))
	}
	for field, selected := range fields {
		if field == nil || !selected || ps6136FieldVar(ownerType, field.Name()) != field || !types.Identical(field.Type(), types.Typ[types.Bool]) {
			return nil
		}
	}
	// A source allocation/call value must not identify multiple dynamic owner
	// instances during this constructor invocation.
	stage = "acyclic owner invocation"
	for current, point := owner.context, ssa.Instruction(allocation); ; current, point = current.parent, current.site {
		if current == nil || point == nil || !ps6140AcyclicInstruction(current, point, &budget) {
			return nil
		}
		if current == constructor {
			break
		}
	}
	var publications []*ssa.Return
	stage = "successful owner publication"
	for _, block := range constructor.flow.function.Blocks {
		if !constructor.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok || len(returned.Results) == 0 {
				continue
			}
			if value, ok := returned.Results[0].(*ssa.Const); ok && value.IsNil() {
				continue
			}
			if !ps6140PublicationInstructionDominates(owner.context, allocation, constructor, returned, owner, ownerType, budget) {
				return nil
			}
			publications = append(publications, returned)
		}
	}
	if len(publications) == 0 {
		return nil
	}
	stores := make(map[*types.Var][]ps6140FlagStore)
	pending := []*ps6125SSAContext{constructor}
	seen := make(map[*ps6125SSAContext]bool)
	for len(pending) != 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == nil || current.flow == nil || budget <= 0 {
			return nil
		}
		budget--
		if seen[current] {
			continue
		}
		seen[current] = true
		for _, block := range current.flow.function.Blocks {
			if !current.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				stage = current.flow.function.String() + ": " + instruction.String()
				budget--
				if budget <= 0 {
					return nil
				}
				switch instruction := instruction.(type) {
				case *ssa.Go, *ssa.Defer:
					return nil // No completion/ordering guarantee is inferred.
				case *ssa.MakeInterface:
					if isOwner(instruction.X.Type()) {
						return nil // Boxing must not hide an opaque owner argument.
					}
				case *ssa.Convert:
					if isOwner(instruction.X.Type()) {
						return nil
					}
				case *ssa.ChangeType:
					if isOwner(instruction.X.Type()) {
						return nil
					}
				case *ssa.FieldAddr:
					if !types.Identical(instruction.X.Type(), types.NewPointer(ownerType)) || instruction.Field < 0 || instruction.Field >= structure.NumFields() || !fields[structure.Field(instruction.Field)] {
						continue
					}
					if instruction.Referrers() == nil {
						return nil
					}
					for _, user := range *instruction.Referrers() {
						switch use := user.(type) {
						case *ssa.DebugRef:
						case *ssa.UnOp:
							if use.Op != token.MUL || use.X != instruction {
								return nil
							}
						case *ssa.Store:
							if use.Addr != instruction || use.Val == instruction {
								return nil
							}
						default:
							return nil // An escaped flag address is not a scalar store.
						}
					}
				case *ssa.Store:
					if isOwner(instruction.Val.Type()) {
						// A local captured variable is analyzed through its actual
						// source calls below. Globals and aggregate retention are
						// not local capture-cell initialization.
						if _, local := instruction.Addr.(*ssa.Alloc); !local {
							return nil
						}
					}
					if types.Identical(instruction.Addr.Type(), types.NewPointer(ownerType)) {
						return nil // Whole-owner replacement is not a field initializer.
					}
					address, ok := instruction.Addr.(*ssa.FieldAddr)
					if !ok || !types.Identical(address.X.Type(), types.NewPointer(ownerType)) || address.Field < 0 || address.Field >= structure.NumFields() || !fields[structure.Field(address.Field)] {
						continue
					}
					root := ps6136OwnerRoot(current, address.X, ownerType)
					if root.value == nil {
						return nil
					}
					if root != owner {
						if _, fresh := root.value.(*ssa.Alloc); !fresh {
							return nil
						}
						continue
					}
					value := current.scalar(instruction.Val)
					if value.state != ps6125Boolean {
						return nil
					}
					field := structure.Field(address.Field)
					stores[field] = append(stores[field], ps6140FlagStore{current, instruction, value.truth})
				case *ssa.Call:
					if child := ps6136Call(current, instruction); child != nil {
						pending = append(pending, child)
						continue
					}
					if callee := instruction.Call.StaticCallee(); callee != nil && callee.Pkg == constructor.flow.function.Pkg && len(callee.Blocks) != 0 {
						return nil // An unresolved local body must not disappear.
					}
					for _, argument := range instruction.Call.Args {
						if ps6140OpaqueOwnerArgument(argument.Type(), ownerType, make(map[types.Type]bool), 128) {
							return nil
						}
					}
				}
			}
		}
	}
	stage = "final flag stores"
	snapshot := &ps6140ConstructorFlagSnapshot{constructor: constructor, owner: owner, values: make(map[*types.Var]bool)}
	for field := range fields {
		observations := stores[field]
		truth := false // Actual fresh Go allocation's initial boolean value.
		for index, observation := range observations {
			if index != 0 && truth != observation.value {
				return nil // Conflicting stores need a stronger ordered-state proof.
			}
			truth = observation.value
		}
		if truth {
			for _, publication := range publications {
				initialized := false
				for _, observation := range observations {
					initialized = initialized || ps6140PublicationInstructionDominates(observation.context, observation.store, constructor, publication, owner, ownerType, budget)
				}
				if !initialized {
					return nil
				}
			}
		}
		snapshot.values[field] = truth
	}
	return snapshot
}

// Unknown callbacks may hide owner captures; pointers and aggregates may hide
// an owner argument even when their immediate type differs from *Owner.
// Interface boxing is checked at its source instruction, not assumed from a
// foreign interface's dynamic type. The full effect closure remains required.
func ps6140OpaqueOwnerArgument(typ types.Type, owner *types.Named, seen map[types.Type]bool, budget int) bool {
	if typ == nil || budget <= 0 {
		return true
	}
	if types.Identical(typ, owner) {
		return true
	}
	if seen[typ] {
		return false
	}
	seen[typ] = true
	switch typ := typ.Underlying().(type) {
	case *types.Pointer:
		return ps6140OpaqueOwnerArgument(typ.Elem(), owner, seen, budget-1)
	case *types.Slice:
		return ps6140OpaqueOwnerArgument(typ.Elem(), owner, seen, budget-1)
	case *types.Array:
		return ps6140OpaqueOwnerArgument(typ.Elem(), owner, seen, budget-1)
	case *types.Map:
		return ps6140OpaqueOwnerArgument(typ.Key(), owner, seen, budget-1) || ps6140OpaqueOwnerArgument(typ.Elem(), owner, seen, budget-1)
	case *types.Chan:
		return ps6140OpaqueOwnerArgument(typ.Elem(), owner, seen, budget-1)
	case *types.Struct:
		for index := 0; index < typ.NumFields(); index++ {
			if ps6140OpaqueOwnerArgument(typ.Field(index).Type(), owner, seen, budget-1) {
				return true
			}
		}
	case *types.Signature:
		return true
	}
	return false
}

func ps6140AcyclicInstruction(context *ps6125SSAContext, point ssa.Instruction, remaining *int) bool {
	if context == nil || context.flow == nil || point == nil || remaining == nil || *remaining <= 0 || point.Parent() != context.flow.function || !context.flow.blocks[point.Block()] {
		return false
	}
	target := point.Block()
	queue := slices.Clone(target.Succs)
	seen := make(map[*ssa.BasicBlock]bool)
	for len(queue) != 0 {
		*remaining--
		if *remaining <= 0 {
			return false
		}
		block := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if block == nil || block.Parent() != context.flow.function {
			return false
		}
		if !context.flow.blocks[block] || seen[block] {
			continue
		}
		if block == target {
			return false
		}
		seen[block] = true
		for _, successor := range block.Succs {
			if context.flow.edges[ps6125SSAEdge{from: block, to: successor}] {
				queue = append(queue, successor)
			}
		}
	}
	return true
}
