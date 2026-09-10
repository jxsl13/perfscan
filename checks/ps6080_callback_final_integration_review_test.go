package checks

import (
	"go/types"
	"testing"
)

func TestPS6080CallbackGraphReturnedWrapperIsNotInvoked(t *testing.T) {
	t.Parallel()
	pass := ps6080CallbackGrowthPass(t, `package probe
func retain(cb func()) func() { return cb }
func walk(f func(int)) { _ = retain(func() { f(41) }) }
`)
	graph := ps6080BuildCallbackGraph(pass)
	walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	node := graph.nodes[ps6080CallbackNodeKey{function: walk, parameter: 0}]
	if node == nil || len(node.direct) != 0 || len(node.reachable) != 0 || node.unknown {
		t.Fatalf("return-only dispatcher fabricated wrapper execution: %+v", node)
	}
}
