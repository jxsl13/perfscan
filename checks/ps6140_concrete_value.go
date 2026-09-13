package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6140ConcreteValue proves every executable source return/phi alternative
// boxes the exact value type. It does not prove collection membership,
// post-publication invariance, or a consumer's dispatch receiver association.
func ps6140ConcreteValue(context *ps6125SSAContext, value ssa.Value, concrete *types.Named, budget int) bool {
	proof := ps6140ConcreteProof{concrete: concrete, remaining: budget, active: make(map[ps6125SSAReference]bool), memo: make(map[ps6125SSAReference]bool)}
	return proof.value(context, value)
}

type ps6140ConcreteProof struct {
	concrete  *types.Named
	remaining int
	active    map[ps6125SSAReference]bool
	memo      map[ps6125SSAReference]bool
}

func (proof *ps6140ConcreteProof) value(context *ps6125SSAContext, value ssa.Value) (valid bool) {
	if context == nil || context.flow == nil || value == nil || proof.concrete == nil {
		return false
	}
	key := ps6125SSAReference{context: context, value: value}
	if result, found := proof.memo[key]; found {
		return result
	}
	if proof.remaining <= 0 || proof.active[key] {
		return false
	}
	proof.remaining--
	proof.active[key] = true
	defer func() { delete(proof.active, key); proof.memo[key] = valid }()
	// reference requires identical SSA values across phi edges. Concrete
	// dispatch instead permits different values of the same concrete type.
	if phi, ok := value.(*ssa.Phi); ok {
		if phi.Parent() != context.flow.function || !context.flow.blocks[phi.Block()] {
			return false
		}
		found := false
		for index, predecessor := range phi.Block().Preds {
			if !context.flow.edges[ps6125SSAEdge{from: predecessor, to: phi.Block()}] {
				continue
			}
			if !proof.value(context, phi.Edges[index]) {
				return false
			}
			found = true
		}
		return found
	}
	reference := context.reference(value)
	if reference.value == nil {
		return false
	}
	if reference.context != context || reference.value != value {
		return proof.value(reference.context, reference.value)
	}
	if boxed, ok := value.(*ssa.MakeInterface); ok {
		return proof.value(context, boxed.X)
	}
	if call, ok := value.(*ssa.Call); ok {
		child := ps6136ZeroSourceFlow(ps6136Call(context, call))
		if child == nil {
			return false
		}
		found := false
		for _, block := range child.flow.function.Blocks {
			if !child.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				ret, ok := instruction.(*ssa.Return)
				if !ok {
					continue
				}
				if len(ret.Results) != 1 || !proof.value(child, ret.Results[0]) {
					return false
				}
				found = true
			}
		}
		return found
	}
	// Exact value type proves dispatch, not the contents of its fields.
	return types.Identical(value.Type(), proof.concrete)
}
