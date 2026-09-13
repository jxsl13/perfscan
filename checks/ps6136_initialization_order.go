package checks

import "golang.org/x/tools/go/ssa"

// ps6136ContextInstructionDominates orders exact source invocation contexts.
// A callee write is lifted to its call only if it dominates every executable
// return; an ancestor write must dominate the actual descendant invocation.
// Unknown completion paths are not an initialization guarantee.
func ps6136ContextInstructionDominates(beforeContext *ps6125SSAContext, before ssa.Instruction, afterContext *ps6125SSAContext, after ssa.Instruction, remaining int) bool {
	if beforeContext == nil || afterContext == nil || beforeContext.flow == nil || afterContext.flow == nil || before == nil || after == nil || before.Parent() != beforeContext.flow.function || after.Parent() != afterContext.flow.function || !beforeContext.flow.blocks[before.Block()] || !afterContext.flow.blocks[after.Block()] || remaining <= 0 {
		return false
	}
	points := make(map[*ps6125SSAContext]ssa.Instruction)
	for current, point := afterContext, after; current != nil; current, point = current.parent, current.site {
		remaining--
		if remaining <= 0 {
			return false
		}
		points[current] = point
	}
	for current, point := beforeContext, before; current != nil; current, point = current.parent, current.site {
		remaining--
		if remaining <= 0 {
			return false
		}
		if target := points[current]; target != nil {
			return current.flow.instructionDominates(point, target)
		}
		returned := false
		for _, block := range current.flow.function.Blocks {
			if !current.flow.blocks[block] {
				continue
			}
			for _, instruction := range block.Instrs {
				if _, ok := instruction.(*ssa.Return); ok {
					if !current.flow.instructionDominates(point, instruction) {
						return false
					}
					returned = true
				}
			}
		}
		if !returned || current.site == nil {
			return false
		}
	}
	return false
}
