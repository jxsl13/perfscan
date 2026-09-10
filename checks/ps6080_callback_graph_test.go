package checks

import (
	"fmt"
	"go/types"
	"strings"
	"testing"
)

func TestPS6080CallbackGraphFiniteRecursiveReachability(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		source  string
		want    int
		unknown bool
	}{
		{"self recursion", `func walk(f func(), n int) { if n == 0 { f(); return }; walk(f,n-1); walk(f,n-1) }`, 1, false},
		{"mutual recursion", `func walk(f func()) { f(); step(f); step(f) }; func step(f func()) { walk(f) }`, 1, false},
		{"dead branch", `func walk(f func()) { if false { f() } }`, 0, false},
		{"after panic", `func walk(f func()) { panic(1); f() }`, 0, false},
		{"dormant literal", `func walk(f func()) { _ = func() { f() } }`, 0, false},
		{"invoked literal", `func walk(f func()) { func() { f() }() }`, 1, false},
		{"local noop", `func walk(f func()) { noop(f) }; func noop(f func()) {}`, 0, false},
		{"method expression", `type runner struct{}; func (runner) run(f func()) { f() }; func walk(f func()) { runner.run(runner{},f) }`, 1, false},
		{"unknown retains known", `var opaque func(func()); func walk(f func()) { f(); opaque(f) }`, 1, true},
		{"unknown through cycle", `var opaque func(func()); func walk(f func()) { step(f) }; func step(f func()) { opaque(f); walk(f) }`, 0, true},
		{"callback passed to callback", `func walk(f func(), dispatch func(func())) { dispatch(f) }`, 0, true},
		{"overwritten incoming callback", `func walk(f func()) { f = func() {}; f() }`, 0, false},
		{"overwritten before forwarding", `func walk(f func()) { f = func() {}; leaf(f) }; func leaf(f func()) { f() }`, 0, false},
		{"alias snapshot before overwrite", `func walk(f func()) { alias := f; f = func() {}; alias() }`, 1, false},
		{"captured incoming overwrite", `func walk(f func()) { f = func() {}; func() { f() }() }`, 0, false},
		{"returned callback escapes", `func walk(f func()) func() { return f }`, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, padding := range []int{0, 100} {
				var source strings.Builder
				source.WriteString("package probe\n")
				source.WriteString(test.source)
				for index := range padding {
					fmt.Fprintf(&source, "\nfunc unrelated%d() {}", index)
				}
				pass := ps6080CallbackGrowthPass(t, source.String())
				graph := ps6080BuildCallbackGraph(pass)
				function := pass.Pkg.Scope().Lookup("walk").(*types.Func)
				node := graph.nodes[ps6080CallbackNodeKey{function: function, parameter: 0}]
				if node == nil {
					t.Fatal("callback parameter node missing")
				}
				if len(node.reachable) != test.want || node.unknown != test.unknown {
					t.Fatalf("padding %d: reachable=%d unknown=%v, want %d/%v", padding, len(node.reachable), node.unknown, test.want, test.unknown)
				}
			}
		})
	}
}

func TestPS6080CallbackGraphJoinsIncomingParameters(t *testing.T) {
	t.Parallel()
	pass := ps6080CallbackGrowthPass(t, `package probe
func walk(f, g func(), choose bool) { if choose { f = g }; f() }
`)
	graph := ps6080BuildCallbackGraph(pass)
	function := pass.Pkg.Scope().Lookup("walk").(*types.Func)
	for _, parameter := range []int{0, 1} {
		node := graph.nodes[ps6080CallbackNodeKey{function: function, parameter: parameter}]
		if node == nil || len(node.reachable) != 1 {
			t.Errorf("incoming parameter %d may reach the callback invocation: %+v", parameter, node)
		}
	}
}

func TestPS6080CallbackGraphSharesDiamondPaths(t *testing.T) {
	t.Parallel()
	const depth = 48
	var source strings.Builder
	source.WriteString("package probe\nfunc leaf(f func()) { f() }\n")
	for index := range depth {
		next := "leaf"
		if index > 0 {
			next = fmt.Sprintf("level%d", index-1)
		}
		fmt.Fprintf(&source, "func level%d(f func()) { %s(f); %s(f) }\n", index, next, next)
	}
	pass := ps6080CallbackGrowthPass(t, source.String())
	graph := ps6080BuildCallbackGraph(pass)
	if len(graph.nodes) != depth+1 {
		t.Fatalf("expected one node per callback parameter, got %d", len(graph.nodes))
	}
	edges, terminals, references := 0, 0, 0
	for _, node := range graph.nodes {
		edges += len(node.edges)
		terminals += len(node.direct)
		references += len(node.reachable)
		if len(node.reachable) != 1 || node.unknown {
			t.Fatalf("shared terminal lost at %s: reachable=%d unknown=%v", node.key.function.Name(), len(node.reachable), node.unknown)
		}
	}
	if edges != 2*depth || terminals != 1 || references != depth+1 {
		t.Fatalf("2^%d paths must share a linear graph, got edges=%d terminals=%d references=%d", depth, edges, terminals, references)
	}
}
