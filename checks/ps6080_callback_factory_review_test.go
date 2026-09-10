package checks

import (
	"go/types"
	"testing"
)

func TestPS6080CallbackFactoryAllocationDoesNotInvokeCapture(t *testing.T) {
	t.Parallel()
	pass := ps6080CallbackGrowthPass(t, `package probe
type runner struct { callback func() }
func (r *runner) invoke() { r.callback() }
func complete() {}
func walk(f func()) func() {
 r := &runner{callback:f}
 method := r.invoke
 r.callback = complete
 return method
}
`)
	walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	graph := ps6080BuildCallbackGraph(pass)
	node := graph.nodes[ps6080CallbackNodeKey{function: walk, parameter: 0}]
	if len(node.reachable) != 0 || node.opaque {
		t.Fatalf("local allocation must not fabricate an opaque callback invocation: %+v", node)
	}
}
