package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// A reference keeps the source invocation context of an SSA value. Two calls
// to the same helper must not conflate that helper's parameters or allocations.
// A source context is not a dynamic instance: one call site may execute in a
// loop, and these references do not prove memory invariance or object lifetime.
type ps6125SSAReference struct {
	context *ps6125SSAContext
	value   ssa.Value
}

type ps6125SSAContextBudget struct{ remaining int }

// Contexts are created on demand for exact callees, with an explicit shared
// allocation budget. Recursion, unresolved targets and exhausted budgets return
// unknown. No whole-program completeness follows from the contexts queried.
type ps6125SSAContext struct {
	flow      *ps6125SSAExtents
	parent    *ps6125SSAContext
	site      *ssa.Call
	budget    *ps6125SSAContextBudget
	bindings  map[ssa.Value]ps6125SSAReference
	calls     map[*ssa.Call]*ps6125SSAContext
	resolved  map[ssa.Value]ps6125SSAReference
	resolving map[ssa.Value]bool
	cells     map[*ssa.Alloc]*ssa.Store
}

func ps6125NewSSAContext(function *ssa.Function, inputs map[*ssa.Parameter]ps6125Scalar, lengths map[*ssa.Parameter]ps6125Extent, limit int) *ps6125SSAContext {
	if function == nil || len(function.Blocks) == 0 || limit <= 0 {
		return nil
	}
	ownedLengths := make(map[*ssa.Parameter]ps6125Extent, len(function.Params))
	for _, parameter := range function.Params {
		if length, exists := lengths[parameter]; exists {
			ownedLengths[parameter] = length
		}
	}
	return &ps6125SSAContext{
		flow:   ps6125AnalyzeSSAExtents(function, inputs, ownedLengths),
		budget: &ps6125SSAContextBudget{remaining: limit - 1},
	}
}

// reference normalizes only parameters, captured cells and executable phi
// inputs. Loads and call results retain their own source sites; neither is
// dereferenced or replaced by a guessed pointee/return value.
func (context *ps6125SSAContext) reference(value ssa.Value) ps6125SSAReference {
	if context == nil || value == nil {
		return ps6125SSAReference{}
	}
	if instruction, local := value.(ssa.Instruction); local && (instruction.Parent() != context.flow.function || !context.flow.blocks[instruction.Block()]) {
		return ps6125SSAReference{}
	}
	switch value.(type) {
	case *ssa.Parameter, *ssa.FreeVar:
		if value.Parent() != context.flow.function {
			return ps6125SSAReference{}
		}
	}
	if context.resolved == nil {
		context.resolved = make(map[ssa.Value]ps6125SSAReference)
		context.resolving = make(map[ssa.Value]bool)
	}
	if result, cached := context.resolved[value]; cached {
		return result
	}
	if context.resolving[value] {
		return ps6125SSAReference{}
	}
	context.resolving[value] = true
	result := ps6125SSAReference{context: context, value: value}
	if bound, exists := context.bindings[value]; exists {
		result = bound.context.reference(bound.value)
	} else if phi, joined := value.(*ssa.Phi); joined {
		result = ps6125SSAReference{}
		for index, predecessor := range phi.Block().Preds {
			if !context.flow.edges[ps6125SSAEdge{from: predecessor, to: phi.Block()}] {
				continue
			}
			incoming := context.reference(phi.Edges[index])
			if incoming.value == nil || result.value != nil && result != incoming {
				result = ps6125SSAReference{}
				break
			}
			result = incoming
		}
	}
	delete(context.resolving, value)
	context.resolved[value] = result
	return result
}

func (context *ps6125SSAContext) scalar(value ssa.Value) ps6125Scalar {
	if fact := context.flow.scalar(value); fact.state == ps6125Integer || fact.state == ps6125Boolean {
		return fact
	}
	reference := context.reference(value)
	if reference.value != nil {
		return reference.context.flow.scalar(reference.value)
	}
	return ps6125Scalar{state: ps6125Unknown}
}

func (context *ps6125SSAContext) length(value ssa.Value) ps6125Extent {
	// Descriptor lengths are stable; mutable map/channel object lengths are not.
	switch typ := value.Type().Underlying().(type) {
	case *types.Slice:
	case *types.Basic:
		if typ.Info()&types.IsString == 0 {
			return ps6125Extent{}
		}
	default:
		return ps6125Extent{}
	}
	reference := context.reference(value)
	switch source := reference.value.(type) {
	case *ssa.Parameter:
		return reference.context.flow.lengths[source]
	case *ssa.MakeSlice:
		fact := reference.context.scalar(source.Len)
		if fact.state == ps6125Integer {
			return fact.extent
		}
	}
	return ps6125Extent{}
}

func (context *ps6125SSAContext) call(call *ssa.Call) *ps6125SSAContext {
	if context == nil || call == nil || call.Parent() != context.flow.function || !context.flow.blocks[call.Block()] || call.Call.IsInvoke() {
		return nil
	}
	if context.calls == nil {
		context.calls = make(map[*ssa.Call]*ps6125SSAContext)
	}
	if child, queried := context.calls[call]; queried {
		return child
	}
	context.calls[call] = nil
	origin := context.callable(call.Call.Value)
	function, captures := ps6125SSACallee(origin.value)
	if function == nil || len(function.Params) != len(call.Call.Args) || context.budget.remaining == 0 {
		return nil
	}
	for ancestor := context; ancestor != nil; ancestor = ancestor.parent {
		if ancestor.flow.function == function {
			return nil
		}
	}
	inputs := make(map[*ssa.Parameter]ps6125Scalar, len(function.Params))
	lengths := make(map[*ssa.Parameter]ps6125Extent, len(function.Params))
	bindings := make(map[ssa.Value]ps6125SSAReference, len(function.Params)+len(captures))
	for index, parameter := range function.Params {
		argument := call.Call.Args[index]
		bindings[parameter] = ps6125SSAReference{context: context, value: argument}
		inputs[parameter] = context.scalar(argument)
		lengths[parameter] = context.length(argument)
	}
	for index, captured := range function.FreeVars {
		// A closure passed through another helper still captures cells from
		// its creating context, not cells in the helper that invoked it.
		bindings[captured] = ps6125SSAReference{context: origin.context, value: captures[index]}
	}
	context.budget.remaining--
	child := &ps6125SSAContext{
		flow:     ps6125AnalyzeSSAExtents(function, inputs, lengths),
		parent:   context,
		site:     call,
		budget:   context.budget,
		bindings: bindings,
	}
	context.calls[call] = child
	return child
}

func (context *ps6125SSAContext) returnedFields(returned *ssa.Return, index int) (ps6125SSAReference, map[*types.Var]ps6125SSAReference, bool) {
	if context == nil {
		return ps6125SSAReference{}, nil, false
	}
	origins := ps6125SSAOrigins{flow: context.flow}
	allocation, fields, known := origins.returnedFields(returned, index)
	if !known {
		return ps6125SSAReference{}, nil, false
	}
	values := make(map[*types.Var]ps6125SSAReference, len(fields))
	for field, value := range fields {
		if reference := context.reference(value); reference.value != nil {
			values[field] = reference
		}
	}
	return ps6125SSAReference{context: context, value: allocation}, values, true
}
