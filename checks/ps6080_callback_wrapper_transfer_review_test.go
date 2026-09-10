package checks

import (
	"go/types"
	"testing"
)

func TestPS6080CallbackGraphWrapperForwarding(t *testing.T) {
	t.Parallel()
	pass := ps6080CallbackGrowthPass(t, `package probe
func dispatch(cb func()) { cb() }
func leaf(cb func(int)) { cb(23) }
func walk(f func(int)) { dispatch(func() { leaf(f) }) }
`)
	graph := ps6080BuildCallbackGraph(pass)
	walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	node := graph.nodes[ps6080CallbackNodeKey{function: walk, parameter: 0}]
	if node == nil || len(node.reachable) != 1 || node.unknown {
		t.Fatalf("wrapper forwarding lost finite callback terminal: %+v", node)
	}
}

func TestPS6080CallbackGraphWrapperOpaqueEscape(t *testing.T) {
	t.Parallel()
	pass := ps6080CallbackGrowthPass(t, `package probe
func dispatch(cb func()) { cb() }
func sink(value any) {}
func walk(f func(int)) { dispatch(func() { sink(f) }) }
`)
	graph := ps6080BuildCallbackGraph(pass)
	walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	node := graph.nodes[ps6080CallbackNodeKey{function: walk, parameter: 0}]
	if node == nil || !node.unknown || !node.opaque {
		t.Fatalf("wrapper opaque argument lost callback escape: %+v", node)
	}
}

func TestPS6080CallbackGraphReusedWrapperLiteralInternsTerminal(t *testing.T) {
	t.Parallel()
	pass := ps6080CallbackGrowthPass(t, `package probe
func dispatch(cb func()) { cb() }
func walk(f func(int)) {
	wrapper := func() { f(23) }
	dispatch(wrapper)
	dispatch(wrapper)
}
`)
	graph := ps6080BuildCallbackGraph(pass)
	walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	node := graph.nodes[ps6080CallbackNodeKey{function: walk, parameter: 0}]
	if node == nil || len(node.direct) != 1 || len(node.reachable) != 1 || node.unknown {
		t.Fatalf("reused wrapper duplicated its source terminal: %+v", node)
	}
}
