package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Complete workspace-use closure for one conditional method entry, not all
// externally reachable owner methods. Allocation, native retention/release,
// external observers and the full public entry inventory remain independent.
type ps6140ScenarioUses struct {
	scenario  *ps6140ConstructorScenario
	workspace *types.Var
	reads     int
	leaves    int
}

type ps6140ResidualUse struct {
	reference ps6125SSAReference
	phase     uint8 // owner field address, slot pointer, slot.b address, buffer
}

type ps6140ResidualGraph struct {
	scenario  *ps6140ConstructorScenario
	slot      *types.Var
	formal    int
	leaves    map[*ps6125SSAContext]map[*ssa.Call]bool
	seen      map[ps6140ResidualUse]bool
	remaining int
}

func ps6140ScenarioWorkspaceUses(scenario *ps6140ConstructorScenario, workspace, slot *types.Var, formal, budget int) *ps6140ScenarioUses {
	result := ps6140ScenarioWorkspaceClosure(scenario, workspace, slot, formal, budget)
	if result == nil || result.reads == 0 || result.leaves == 0 {
		return nil
	}
	return result
}

// Empty closure is useful for public query/release methods with no direct
// workspace reads. A candidate still requires genuine unused-formal witnesses
// across the complete entry inventory; emptiness alone is never a finding.
func ps6140ScenarioWorkspaceClosure(scenario *ps6140ConstructorScenario, workspace, slot *types.Var, formal, budget int) *ps6140ScenarioUses {
	return ps6140ScenarioWorkspaceClosureCheck(scenario, workspace, slot, formal, budget, nil)
}

func ps6140ScenarioWorkspaceClosureCheck(scenario *ps6140ConstructorScenario, workspace, slot *types.Var, formal, budget int, rejected func(string)) (result *ps6140ScenarioUses) {
	stage := "input"
	defer func() {
		if result == nil && rejected != nil {
			rejected(stage)
		}
	}()
	if scenario == nil || workspace == nil || slot == nil || formal < 0 || budget <= 0 || ps6136FieldVar(scenario.ownerType, workspace.Name()) != workspace || ps6136FieldVar(workspace.Type(), slot.Name()) != slot {
		return nil
	}
	block, ok := scenario.collection.appended.value.Type().(*types.Named)
	if !ok {
		return nil
	}
	graph := &ps6140ResidualGraph{scenario: scenario, slot: slot, formal: formal, leaves: make(map[*ps6125SSAContext]map[*ssa.Call]bool), seen: make(map[ps6140ResidualUse]bool), remaining: budget}
	proof := &ps6140ScenarioUses{scenario: scenario, workspace: workspace}
	stage = "scenario invocation walk"
	if !scenario.walk(scenario.root, func(current *ps6125SSAContext, call *ssa.Call) bool {
		if !call.Call.IsInvoke() {
			return false
		}
		for projection, concrete := range scenario.collection.projectors {
			if ps6140UnusedProjectionFormal(current, call, scenario.collection.appended, concrete, scenario.receiver, scenario.ownerType, block, scenario.collection.blocks, projection, workspace, slot, formal, 256) != nil {
				if graph.leaves[current] == nil {
					graph.leaves[current] = make(map[*ssa.Call]bool)
				}
				graph.leaves[current][call] = true
				proof.leaves++
				return true
			}
		}
		return false
	}) {
		return nil
	}
	structure := scenario.ownerType.Underlying().(*types.Struct)
	callbacks := &ps6140ScenarioCallbacks{contexts: scenario.refined, ownerType: scenario.ownerType, workspace: workspace, remaining: budget, active: make(map[ps6140ScenarioCallableUse]bool), done: make(map[ps6140ScenarioCallableUse]bool)}
	for context := range scenario.refined {
		for _, bb := range context.flow.function.Blocks {
			if !context.flow.blocks[bb] {
				continue
			}
			for _, instruction := range bb.Instrs {
				stage = context.flow.function.String() + ": " + instruction.String()
				graph.remaining--
				if graph.remaining <= 0 {
					return nil
				}
				if closure, ok := instruction.(*ssa.MakeClosure); ok && !callbacks.closure(context, closure) {
					return nil
				}
				if call, ok := instruction.(ssa.CallInstruction); ok && !callbacks.ownerCall(context, call) {
					return nil
				}
				address, ok := instruction.(*ssa.FieldAddr)
				if !ok || !types.Identical(address.X.Type(), types.NewPointer(scenario.ownerType)) || address.Field < 0 || address.Field >= structure.NumFields() || structure.Field(address.Field) != workspace {
					continue
				}
				// A foreign/unknown parameter may alias the selected object.
				// Distinct SSA references alone are not a disjointness proof.
				if ps6136OwnerRoot(context, address.X, scenario.ownerType) != scenario.receiver || !graph.close(ps6140ResidualUse{reference: ps6125SSAReference{context: context, value: address}}) {
					return nil
				}
				proof.reads++
			}
		}
	}
	return proof
}

func (graph *ps6140ResidualGraph) close(use ps6140ResidualUse) bool {
	context, value := use.reference.context, use.reference.value
	if context == nil || value == nil || !graph.scenario.refined[context] || graph.remaining <= 0 {
		return false
	}
	graph.remaining--
	if graph.seen[use] {
		return true
	}
	graph.seen[use] = true
	users := value.Referrers()
	if users == nil {
		return false
	}
	for _, instruction := range *users {
		graph.remaining--
		if graph.remaining <= 0 || instruction.Parent() != context.flow.function {
			return false
		}
		if !context.flow.blocks[instruction.Block()] {
			continue
		}
		next := use
		next.reference.value, _ = instruction.(ssa.Value)
		switch user := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.UnOp:
			if user.Op != token.MUL || user.X != value || use.phase != 0 && use.phase != 2 {
				return false
			}
			next.phase++
			if !graph.close(next) {
				return false
			}
		case *ssa.FieldAddr:
			if use.phase != 1 || user.X != value {
				return false
			}
			pointer, ok := value.Type().Underlying().(*types.Pointer)
			if !ok {
				return false
			}
			structure, ok := pointer.Elem().Underlying().(*types.Struct)
			if !ok || user.Field < 0 || user.Field >= structure.NumFields() || structure.Field(user.Field) != graph.slot {
				return false
			}
			next.phase = 2
			if !graph.close(next) {
				return false
			}
		case *ssa.Call:
			if user.Call.Value == value {
				return false // Calling a workspace method observes its buffer.
			}
			if use.phase == 3 && graph.leaves[context][user] {
				found := false
				for index, argument := range user.Call.Args {
					if argument == value {
						if index != graph.formal {
							return false // Same buffer in another, used formal.
						}
						found = true
					}
				}
				if !found {
					return false
				}
				continue
			}
			child := ps6136Call(context, user)
			if child == nil || !graph.scenario.refined[child] || len(child.flow.function.Params) != len(user.Call.Args) {
				return false
			}
			found := false
			for index, argument := range user.Call.Args {
				if argument == value {
					found = true
					if !graph.close(ps6140ResidualUse{reference: ps6125SSAReference{context: child, value: child.flow.function.Params[index]}, phase: use.phase}) {
						return false
					}
				}
			}
			if !found {
				return false
			}
		default:
			return false // stores, aliases, phis, returns, captures, go/defer
		}
	}
	return true
}
