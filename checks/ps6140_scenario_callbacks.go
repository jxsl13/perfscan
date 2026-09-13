package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// A captured owner can hide future workspace observations without loading a
// workspace in the enclosing method. Close the callable's uses, including
// source forwarding and private function cells, before trusting that method's
// invocation census. This grants no native effect or buffer-alias exemption.
type ps6140ScenarioCallableUse struct {
	reference ps6125SSAReference
	cell      bool
}

type ps6140ScenarioCallbacks struct {
	contexts  map[*ps6125SSAContext]bool
	ownerType *types.Named
	workspace *types.Var
	active    map[ps6140ScenarioCallableUse]bool
	done      map[ps6140ScenarioCallableUse]bool
	remaining int
	rejected  func(string)
}

func (graph *ps6140ScenarioCallbacks) close(use ps6140ScenarioCallableUse) (result bool) {
	stage := "callback input"
	defer func() {
		if !result && graph.rejected != nil {
			graph.rejected(stage)
		}
	}()
	context, value := use.reference.context, use.reference.value
	if context == nil || value == nil || !graph.contexts[context] || graph.remaining <= 0 || graph.active[use] {
		return false
	}
	graph.remaining--
	if graph.done[use] {
		return true
	}
	users := value.Referrers()
	if users == nil {
		return false
	}
	graph.active[use] = true
	defer delete(graph.active, use)
	for _, instruction := range *users {
		stage = context.flow.function.String() + ": " + instruction.String()
		graph.remaining--
		if graph.remaining <= 0 || instruction.Parent() != context.flow.function {
			return false
		}
		if !context.flow.blocks[instruction.Block()] {
			continue
		}
		switch user := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.Call:
			child := context.calls[user]
			if child == nil || !graph.contexts[child] || child.parent != context || child.site != user || len(child.flow.function.Params) != len(user.Call.Args) {
				return false
			}
			found := !use.cell && user.Call.Value == value
			for index, argument := range user.Call.Args {
				if argument == value {
					found = true
					if !graph.close(ps6140ScenarioCallableUse{reference: ps6125SSAReference{context: child, value: child.flow.function.Params[index]}, cell: use.cell}) {
						return false
					}
				}
			}
			if !found {
				return false
			}
		case *ssa.Defer:
			if use.cell || user.Call.Value != value || !ps6140DeferredFieldRestoration(context, user, graph.ownerType, graph.workspace) {
				return false
			}
		case *ssa.Return:
			// A returned callback can remain private to its actual caller.
			// Its caller result must select this same closure on every return;
			// close all uses of that result, including any later publication.
			if use.cell || len(user.Results) != 1 || user.Results[0] != value || context.parent == nil || context.site == nil || !graph.contexts[context.parent] || context.parent.calls[context.site] != context {
				return false
			}
			expected := ps6136ReturnedCallable(context.reference(value), make(map[ps6125SSAReference]bool))
			returned := ps6136ReturnedCallable(context.parent.reference(context.site), make(map[ps6125SSAReference]bool))
			if expected.value == nil || expected != returned || !graph.close(ps6140ScenarioCallableUse{reference: ps6125SSAReference{context: context.parent, value: context.site}}) {
				return false
			}
		case *ssa.Store:
			if use.cell {
				allocation, ok := value.(*ssa.Alloc)
				if !ok || user.Addr != value || user.Val == value || context.functionCell(allocation) != user {
					return false
				}
				continue // The single initializer, not a callable exposure.
			}
			allocation, ok := user.Addr.(*ssa.Alloc)
			if !ok || user.Val != value || context.functionCell(allocation) != user || !graph.close(ps6140ScenarioCallableUse{reference: ps6125SSAReference{context: context, value: allocation}, cell: true}) {
				return false
			}
		case *ssa.UnOp:
			if !use.cell || user.Op != token.MUL || user.X != value || !graph.close(ps6140ScenarioCallableUse{reference: ps6125SSAReference{context: context, value: user}}) {
				return false
			}
		case *ssa.MakeClosure:
			function, captures := ps6125SSACallee(user)
			if function == nil || !graph.close(ps6140ScenarioCallableUse{reference: ps6125SSAReference{context: context, value: user}}) {
				return false
			}
			found := false
			for index, capture := range captures {
				if capture != value {
					continue
				}
				found = true
				// Only exact actual invocations of this closure are relevant.
				// Merely sharing its function body does not share its captures.
				for child := range graph.contexts {
					graph.remaining--
					if graph.remaining <= 0 {
						return false
					}
					if child.flow.function != function || child.parent == nil || child.site == nil {
						continue
					}
					origin := ps6136ReturnedCallable(child.parent.reference(child.site.Call.Value), make(map[ps6125SSAReference]bool))
					if origin != (ps6125SSAReference{context: context, value: user}) {
						continue
					}
					if !graph.close(ps6140ScenarioCallableUse{reference: ps6125SSAReference{context: child, value: function.FreeVars[index]}, cell: use.cell}) {
						return false
					}
				}
			}
			if !found {
				return false
			}
		default:
			return false // Returns, global/container retention, casts, go/defer.
		}
	}
	graph.done[use] = true
	return true
}

func (graph *ps6140ScenarioCallbacks) closure(context *ps6125SSAContext, closure *ssa.MakeClosure) bool {
	for _, capture := range closure.Bindings {
		// Includes captured **Owner cells, aggregates and nested callbacks.
		// Interface boxing/whole-owner exposure is independently closed by
		// the mandatory immutable constructor proof.
		if ps6140OpaqueOwnerArgument(capture.Type(), graph.ownerType, make(map[types.Type]bool), 128) {
			return graph.close(ps6140ScenarioCallableUse{reference: ps6125SSAReference{context: context, value: closure}})
		}
	}
	return true
}

// A source-visible owner call that is deferred, asynchronous or unresolved
// can hide a workspace read just as a retained closure can. Its body must not
// vanish merely because the current method has no workspace FieldAddr for it.
func (graph *ps6140ScenarioCallbacks) ownerCall(context *ps6125SSAContext, instruction ssa.CallInstruction) bool {
	if graph.remaining <= 0 {
		return false
	}
	for _, argument := range instruction.Common().Args {
		// Owner captures are closed at MakeClosure, including subsequent
		// opaque forwarding. An unrelated comparator does not carry an owner
		// merely because its type is a function (e.g. sort.Slice callbacks).
		contains := ps6140ContainsOwner(argument.Type(), graph.ownerType, make(map[types.Type]bool), &graph.remaining)
		if graph.remaining <= 0 {
			return false
		}
		if !contains {
			continue
		}
		call, synchronous := instruction.(*ssa.Call)
		if !synchronous {
			return false
		}
		child := context.calls[call]
		if child == nil || !graph.contexts[child] || child.parent != context || child.site != call {
			return false
		}
	}
	return true
}
