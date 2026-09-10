package checks

import (
	"go/ast"
	"go/types"
	"testing"
)

func TestPS6080CallbackGraphNestedWrapperDependency(t *testing.T) {
	t.Parallel()
	pass := ps6080CallbackGrowthPass(t, `package probe
func dispatch(cb func()) { cb() }
func walk(f func(int)) {
	dispatch(func() {
		dispatch(func() { f(23) })
	})
}
`)
	graph := ps6080BuildCallbackGraph(pass)
	walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	node := graph.nodes[ps6080CallbackNodeKey{function: walk, parameter: 0}]
	if node == nil || len(node.reachable) != 1 || node.unknown {
		t.Fatalf("nested wrapper lost finite callback terminal: %+v", node)
	}
	for site := range node.reachable {
		if site.function.object != walk || len(site.call.Args) != 1 {
			t.Fatalf("nested wrapper substituted dispatcher terminal: %+v", site)
		}
		literal, ok := site.call.Args[0].(*ast.BasicLit)
		if !ok || literal.Value != "23" {
			t.Fatalf("nested wrapper lost actual callback argument: %+v", site.call.Args)
		}
	}
}
