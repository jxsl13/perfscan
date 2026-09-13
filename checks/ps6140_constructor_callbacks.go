package checks

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Close owner-carrying callbacks created during construction, which can publish
// future reads without an exported function accepting an Owner parameter. No
// publication-time flag snapshot is used to prune pre-publication instructions.
// This proves callback closure, not absence of ordinary constructor reads or
// native observations of the retained buffer list.
func ps6140ConstructorCallbacks(constructor *ps6125SSAContext, owner *types.Named, workspace *types.Var, budget int) bool {
	return ps6140ConstructorCallbacksCheck(constructor, owner, workspace, budget, nil)
}

func ps6140ConstructorCallbacksCheck(constructor *ps6125SSAContext, owner *types.Named, workspace *types.Var, budget int, rejected func(string)) (result bool) {
	stage := "input"
	defer func() {
		if !result && rejected != nil {
			rejected(stage)
		}
	}()
	if constructor == nil || constructor.flow == nil || owner == nil || workspace == nil || budget <= 0 {
		return false
	}
	contexts := make(map[*ps6125SSAContext]bool)
	pending := []*ps6125SSAContext{constructor}
	remaining := budget
	for len(pending) != 0 {
		context := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if contexts[context] {
			continue
		}
		contexts[context] = true
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				stage = context.flow.function.String() + ": " + instruction.String()
				remaining--
				if remaining <= 0 {
					return false
				}
				if call, ok := instruction.(*ssa.Call); ok {
					if child := ps6136Call(context, call); child != nil {
						pending = append(pending, child)
					}
				}
			}
		}
	}
	graph := &ps6140ScenarioCallbacks{contexts: contexts, ownerType: owner, workspace: workspace, active: make(map[ps6140ScenarioCallableUse]bool), done: make(map[ps6140ScenarioCallableUse]bool), remaining: budget, rejected: rejected}
	for context := range contexts {
		for _, block := range context.flow.function.Blocks {
			if !context.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				stage = context.flow.function.String() + ": " + instruction.String()
				remaining--
				if remaining <= 0 {
					return false
				}
				if closure, ok := instruction.(*ssa.MakeClosure); ok && !graph.closure(context, closure) {
					return false
				}
				if call, ok := instruction.(ssa.CallInstruction); ok && !graph.ownerCall(context, call) {
					return false
				}
			}
		}
	}
	return true
}
