package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// ps6136Call adds only closed, source-visible returned callable provenance to
// the existing contextual call proof. It does not attest closure ownership or
// purity. All executable returns must resolve to the same callable instance.
func ps6136Call(context *ps6125SSAContext, call *ssa.Call) *ps6125SSAContext {
	if child := context.call(call); child != nil {
		return child
	}
	if context == nil || call == nil || call.Parent() != context.flow.function || !context.flow.blocks[call.Block()] || call.Call.IsInvoke() || context.budget.remaining == 0 {
		return nil
	}
	origin := ps6136ReturnedCallable(context.reference(call.Call.Value), make(map[ps6125SSAReference]bool))
	return ps6136CallOrigin(context, call, origin)
}

// ps6136CallOrigin requires an already source-proved callable identity. It is
// not an opaque-function configuration escape hatch: immutable owner-field
// binding callers must close the source field's complete observation inventory.
func ps6136CallOrigin(context *ps6125SSAContext, call *ssa.Call, origin ps6125SSAReference) *ps6125SSAContext {
	if context == nil || call == nil || origin.context == nil || call.Parent() != context.flow.function || !context.flow.blocks[call.Block()] || call.Call.IsInvoke() || context.budget.remaining == 0 {
		return nil
	}
	function, captures := ps6125SSACallee(origin.value)
	if function == nil || len(function.Params) != len(call.Call.Args) || len(function.FreeVars) != len(captures) || !types.Identical(function.Signature, call.Call.Value.Type()) {
		return nil
	}
	for ancestor := context; ancestor != nil; ancestor = ancestor.parent {
		if ancestor.flow.function == function {
			return nil
		}
	}
	inputs := make(map[*ssa.Parameter]ps6125Scalar, len(function.Params))
	lengths := make(map[*ssa.Parameter]ps6125Extent, len(function.Params))
	bindings := make(map[ssa.Value]ps6125SSAReference, len(function.Params)+len(function.FreeVars))
	for index, parameter := range function.Params {
		argument := call.Call.Args[index]
		bindings[parameter] = context.reference(argument)
		inputs[parameter] = context.scalar(argument)
		lengths[parameter] = context.length(argument)
	}
	for index, capture := range function.FreeVars {
		bindings[capture] = origin.context.reference(captures[index])
	}
	context.budget.remaining--
	child := &ps6125SSAContext{flow: ps6125AnalyzeSSAExtents(function, inputs, lengths), parent: context, site: call, budget: context.budget, bindings: bindings}
	if context.calls == nil {
		context.calls = make(map[*ssa.Call]*ps6125SSAContext)
	}
	context.calls[call] = child
	return child
}

func ps6136ReturnedCallable(reference ps6125SSAReference, active map[ps6125SSAReference]bool) ps6125SSAReference {
	if reference.value == nil || reference.context == nil || active[reference] {
		return ps6125SSAReference{}
	}
	if known := reference.context.callable(reference.value); known.value != nil {
		return known
	}
	active[reference] = true
	defer delete(active, reference)
	if load, ok := reference.value.(*ssa.UnOp); ok && load.Op == token.MUL {
		cell := reference.context.reference(load.X)
		allocation, ok := cell.value.(*ssa.Alloc)
		if !ok || cell.context == nil {
			return ps6125SSAReference{}
		}
		store := cell.context.functionCell(allocation)
		if store == nil {
			return ps6125SSAReference{}
		}
		var point ssa.Instruction = load
		current := reference.context
		for current != cell.context && current.parent != nil {
			point = current.site
			current = current.parent
		}
		if current != cell.context || !cell.context.flow.instructionDominates(store, point) {
			return ps6125SSAReference{}
		}
		return ps6136ReturnedCallable(cell.context.reference(store.Val), active)
	}
	call, ok := reference.value.(*ssa.Call)
	if !ok {
		return ps6125SSAReference{}
	}
	child := ps6136Call(reference.context, call)
	if child == nil {
		return ps6125SSAReference{}
	}
	var result ps6125SSAReference
	for _, block := range child.flow.function.Blocks {
		if !child.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			if len(returned.Results) != 1 {
				return ps6125SSAReference{}
			}
			value := ps6136ReturnedCallable(child.reference(returned.Results[0]), active)
			if value.value == nil || result.value != nil && result != value {
				return ps6125SSAReference{}
			}
			result = value
		}
	}
	return result
}
