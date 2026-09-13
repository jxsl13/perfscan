package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136Extents adds only source-proved immutable owner geometry to the
// invocation-specialized PS6125 algebra. It does not infer field invariance
// from repeated loads; callers must close owner mutation/exposure first.
type ps6136Extents struct {
	owner     ps6125SSAReference
	ownerType *types.Named
	immutable map[*types.Var]ps6125Extent
	cache     map[ps6125SSAReference]ps6125Extent
	active    map[ps6125SSAReference]bool
	remaining int
}

func (facts *ps6136Extents) extent(context *ps6125SSAContext, value ssa.Value) ps6125Extent {
	if context == nil || value == nil || facts.remaining <= 0 || facts.owner.value == nil {
		return ps6125Extent{}
	}
	if scalar := context.scalar(value); scalar.state == ps6125Integer {
		return scalar.extent
	}
	reference := context.reference(value)
	if reference.value == nil {
		return ps6125Extent{}
	}
	if reference.context != context || reference.value != value {
		return facts.extent(reference.context, reference.value)
	}
	if facts.cache == nil {
		facts.cache = make(map[ps6125SSAReference]ps6125Extent)
		facts.active = make(map[ps6125SSAReference]bool)
	}
	if result, found := facts.cache[reference]; found {
		return result
	}
	if facts.active[reference] {
		return ps6125Extent{}
	}
	facts.remaining--
	facts.active[reference] = true
	var result ps6125Extent
	if types.Identical(value.Type(), types.Typ[types.Int]) {
		paths := ps6125AccessPaths{flow: context.flow}
		path := paths.resolve(value)
		root := context.reference(path.access.root)
		if facts.ownerType != nil {
			root = ps6136AccessOwnerRoot(context, path, facts.ownerType)
		}
		if path.known && len(path.access.fields) == 1 && root == facts.owner {
			result = facts.immutable[path.access.fields[0]]
		}
	}
	if !result.known {
		switch instruction := value.(type) {
		case *ssa.BinOp:
			if instruction.Op == token.MUL && types.Identical(instruction.Type(), types.Typ[types.Int]) {
				result = ps6125MultiplyExtents(facts.extent(context, instruction.X), facts.extent(context, instruction.Y))
			}
		case *ssa.Phi:
			found := false
			for index, predecessor := range instruction.Block().Preds {
				if !context.flow.edges[ps6125SSAEdge{from: predecessor, to: instruction.Block()}] {
					continue
				}
				incoming := facts.extent(context, instruction.Edges[index])
				if !incoming.known || found && !ps6125SameExtent(result, incoming) {
					result = ps6125Extent{}
					break
				}
				result, found = incoming, true
			}
		case *ssa.Call:
			if builtin, ok := instruction.Call.Value.(*ssa.Builtin); ok && builtin.Name() == "len" && len(instruction.Call.Args) == 1 {
				result = facts.length(context, instruction.Call.Args[0])
			}
		}
	}
	delete(facts.active, reference)
	facts.cache[reference] = result
	return result
}

func (facts *ps6136Extents) length(context *ps6125SSAContext, value ssa.Value) ps6125Extent {
	if context == nil || value == nil {
		return ps6125Extent{}
	}
	if length := context.length(value); length.known {
		return length
	}
	reference := context.reference(value)
	allocation, ok := reference.value.(*ssa.MakeSlice)
	if !ok {
		return ps6125Extent{}
	}
	return facts.extent(reference.context, allocation.Len)
}
