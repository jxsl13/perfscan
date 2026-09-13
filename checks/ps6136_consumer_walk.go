package checks

import "golang.org/x/tools/go/ssa"

// ps6136WalkConsumerCalls retains invocation-specific branches and arguments.
// A visitor may stop at a separately proved leaf. Unresolved calls are delivered
// to the visitor, not treated as harmless: complete observation closure remains
// mandatory before a finding. Exhausting the traversal budget rejects the walk.
func ps6136WalkConsumerCalls(context *ps6125SSAContext, visit func(*ps6125SSAContext, *ssa.Call) bool, remaining *int) bool {
	if context == nil || context.flow == nil || visit == nil || remaining == nil || *remaining <= 0 {
		return false
	}
	*remaining--
	for _, block := range context.flow.function.Blocks {
		if !context.flow.blocks[block] {
			continue
		}
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok || visit(context, call) {
				continue
			}
			child := ps6136Call(context, call)
			if child != nil && !ps6136WalkConsumerCalls(child, visit, remaining) {
				return false
			}
		}
	}
	return true
}
