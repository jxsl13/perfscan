package checks

import (
	"go/ast"
	"go/types"
	"testing"
)

func TestPS6080CallbackGraphOriginBoundaryReview(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, source string
		want         int
		unknown      bool
	}{
		{"conditional literal overwrite retains incoming", `func walk(f func(), b bool) { if b { f = func() {} }; f() }`, 1, false},
		{"assignment RHS executes before callback kill", `var choose func(func()) func(); func walk(f func()) { f = choose(f) }`, 0, true},
		{"wrapper captures local alias", `func leaf(cb func()) { cb() }; func walk(f func()) { g := f; leaf(func() { g() }) }`, 1, false},
		{"returned wrapper captures alias", `func walk(f func()) func() { return func() { g := f; g() } }`, 0, true},
		{"local field write invokes callback", `type box struct { run func() }; func walk(f func()) { b := box{}; b.run = f; b.run() }`, 1, false},
		{"unkeyed field initialization", `type box struct { run func() }; func walk(f func()) { b := box{f}; b.run() }`, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pass := ps6080CallbackGrowthPass(t, "package probe\n"+test.source)
			graph := ps6080BuildCallbackGraph(pass)
			function := pass.Pkg.Scope().Lookup("walk").(*types.Func)
			node := graph.nodes[ps6080CallbackNodeKey{function: function, parameter: 0}]
			if node == nil || len(node.reachable) != test.want || node.unknown != test.unknown {
				t.Fatalf("got %+v, want %d reachable, unknown %v", node, test.want, test.unknown)
			}
		})
	}
}

func TestPS6080CallbackGraphWrapperKeepsActualArguments(t *testing.T) {
	t.Parallel()
	pass := ps6080CallbackGrowthPass(t, `package probe
func leaf(cb func(int), value int) { cb(value) }
func walk(f func(int), value int) { leaf(func(int) { f(17) }, value) }
`)
	graph := ps6080BuildCallbackGraph(pass)
	walk := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	node := graph.nodes[ps6080CallbackNodeKey{function: walk, parameter: 0}]
	if len(node.reachable) != 1 {
		t.Fatalf("want one source callback call, got %d", len(node.reachable))
	}
	for site := range node.reachable {
		if len(site.call.Args) != 1 {
			t.Fatal("lost actual callback argument")
		}
		literal, ok := site.call.Args[0].(*ast.BasicLit)
		if !ok || literal.Value != "17" || site.function.object != walk {
			t.Fatalf("wrapper callback must retain f(17), not substitute dispatcher's cb(value): %+v", site)
		}
	}
}
