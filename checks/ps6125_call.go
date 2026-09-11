package checks

import "golang.org/x/tools/go/ssa"

type ps6125SSAOrigin struct {
	value ssa.Value
	known bool
}

// Origins select an exact SSA value through executable phi inputs. This does
// not dereference memory, interpret call results, or equate repeated loads.
// A resolver belongs to one completed invocation analysis; its executable edge
// set must not change while cached answers are in use.
type ps6125SSAOrigins struct {
	flow   *ps6125SSAExtents
	cache  map[ssa.Value]ps6125SSAOrigin
	active map[ssa.Value]bool
}

func (origins *ps6125SSAOrigins) resolve(value ssa.Value) ps6125SSAOrigin {
	if value == nil {
		return ps6125SSAOrigin{}
	}
	phi, isPhi := value.(*ssa.Phi)
	if !isPhi {
		return ps6125SSAOrigin{value: value, known: true}
	}
	if origins.flow == nil || phi.Parent() != origins.flow.function || !origins.flow.blocks[phi.Block()] {
		return ps6125SSAOrigin{}
	}
	if origins.cache == nil {
		origins.cache = make(map[ssa.Value]ps6125SSAOrigin)
		origins.active = make(map[ssa.Value]bool)
	}
	if result, ok := origins.cache[value]; ok {
		return result
	}
	if origins.active[value] {
		return ps6125SSAOrigin{}
	}
	origins.active[value] = true
	var result ps6125SSAOrigin
	for index, predecessor := range phi.Block().Preds {
		if !origins.flow.edges[ps6125SSAEdge{from: predecessor, to: phi.Block()}] {
			continue
		}
		incoming := origins.resolve(phi.Edges[index])
		if !incoming.known || result.known && result.value != incoming.value {
			result = ps6125SSAOrigin{}
			break
		}
		result = incoming
	}
	delete(origins.active, value)
	origins.cache[value] = result
	return result
}

// A binding records actual SSA arguments and captured cells, not the values
// currently stored in those cells. In particular, binding a free variable to
// &owner does not establish owner identity, freshness, or invariant fields.
// Source SSA values are not dynamic allocation-instance identities in loops.
type ps6125SSACallBinding struct {
	function *ssa.Function
	values   map[ssa.Value]ssa.Value
}

// call accepts only a source-visible function or exact closure value selected
// by the invocation's executable edges. Function parameters, field loads,
// interface invokes, opaque calls and conflicting callable origins are unknown.
func (origins *ps6125SSAOrigins) call(call *ssa.Call) (ps6125SSACallBinding, bool) {
	if origins.flow == nil || call == nil || call.Parent() != origins.flow.function || !origins.flow.blocks[call.Block()] || call.Call.IsInvoke() {
		return ps6125SSACallBinding{}, false
	}
	resolved := origins.resolve(call.Call.Value)
	if !resolved.known {
		return ps6125SSACallBinding{}, false
	}
	var function *ssa.Function
	var captures []ssa.Value
	switch callable := resolved.value.(type) {
	case *ssa.Function:
		function = callable
	case *ssa.MakeClosure:
		function, _ = callable.Fn.(*ssa.Function)
		captures = callable.Bindings
	default:
		return ps6125SSACallBinding{}, false
	}
	if function == nil || len(function.Blocks) == 0 || len(function.Params) != len(call.Call.Args) || len(function.FreeVars) != len(captures) {
		return ps6125SSACallBinding{}, false
	}
	values := make(map[ssa.Value]ssa.Value, len(function.Params)+len(function.FreeVars))
	for index, parameter := range function.Params {
		values[parameter] = call.Call.Args[index]
	}
	for index, captured := range function.FreeVars {
		values[captured] = captures[index]
	}
	return ps6125SSACallBinding{function: function, values: values}, true
}
