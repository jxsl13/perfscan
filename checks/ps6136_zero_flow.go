package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136ZeroSourceFlow refines only CFG edges whose condition is established
// by a closed source struct's nil function/false bool. Unknown heap loads,
// pointer escapes and unresolved callbacks retain both branches. It does not
// establish immutable owner fields or source-call effects.
func ps6136ZeroSourceFlow(context *ps6125SSAContext) *ps6125SSAContext {
	if context == nil || context.flow == nil || len(context.flow.function.Blocks) == 0 {
		return context
	}
	flow := *context.flow
	flow.blocks = make(map[*ssa.BasicBlock]bool)
	flow.edges = make(map[ps6125SSAEdge]bool)
	flow.origins = nil
	clone := &ps6125SSAContext{flow: &flow, parent: context.parent, site: context.site, budget: context.budget, bindings: context.bindings}
	pending := []*ssa.BasicBlock{flow.function.Blocks[0]}
	flow.edges[ps6125SSAEdge{to: flow.function.Blocks[0]}] = true
	for len(pending) != 0 {
		block := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if flow.blocks[block] {
			continue
		}
		flow.blocks[block] = true
		successors := block.Succs
		if len(block.Instrs) > 0 {
			if condition, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If); ok {
				fact := context.scalar(condition.Cond)
				truth, known := fact.truth, fact.state == ps6125Boolean
				if !known {
					truth, known = ps6136ZeroCondition(context, condition.Cond)
				}
				if known {
					index := 1
					if truth {
						index = 0
					}
					successors = successors[index : index+1]
				}
			}
		}
		for _, next := range successors {
			flow.edges[ps6125SSAEdge{from: block, to: next}] = true
			pending = append(pending, next)
		}
	}
	return clone
}

func ps6136ZeroCondition(context *ps6125SSAContext, value ssa.Value) (bool, bool) {
	if types.Identical(value.Type(), types.Typ[types.Bool]) && ps6136ZeroField(context, value) {
		return false, true
	}
	comparison, ok := value.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	left, right := comparison.X, comparison.Y
	if literal, ok := left.(*ssa.Const); ok && literal.IsNil() {
		left, right = right, left
	}
	literal, ok := right.(*ssa.Const)
	_, function := left.Type().Underlying().(*types.Signature)
	if !ok || !literal.IsNil() || !function || !ps6136ZeroField(context, left) {
		return false, false
	}
	return comparison.Op == token.EQL, true
}

func ps6136ZeroField(context *ps6125SSAContext, value ssa.Value) bool {
	if field, ok := value.(*ssa.Field); ok {
		return ps6136StructFieldZero(context, field.X, field.Field, 64)
	}
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	field, ok := load.X.(*ssa.FieldAddr)
	return ok && ps6136StructFieldZeroAt(context, field.X, field.Field, load, 64)
}
