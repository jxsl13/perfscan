package checks

import (
	"go/types"
	"testing"
)

func TestPS6080CallbackGraphIndependentValueFlowReview(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		source  string
		want    int
		unknown bool
	}{
		{
			name: "forwarded wrapper literal capture",
			source: `func leaf(cb func()) { cb() }
func walk(f func()) { leaf(func() { f() }) }`,
			want: 1,
		},
		{
			name: "local aggregate callback alias",
			source: `type callbacks struct { run func() }
func walk(f func()) { box := callbacks{run: f}; box.run() }`,
			want: 1,
		},
		{
			name: "aggregate escape is unknown",
			source: `type callbacks struct { run func() }
var opaque func(callbacks)
func walk(f func()) { opaque(callbacks{run: f}) }`,
			unknown: true,
		},
		{
			name: "deferred capture observes later kill",
			source: `func walk(f func()) {
 run := func() { f() }
 defer run()
 f = func() {}
}`,
			want: 0,
		},
		{
			name: "deferred snapshot retains incoming",
			source: `func walk(f func()) {
 snapshot := f
 defer func() { snapshot() }()
 f = func() {}
}`,
			want: 1,
		},
		{
			name: "goroutine capture may run before kill",
			source: `func walk(f func()) {
 run := func() { f() }
 go run()
 f = func() {}
}`,
			want: 1,
		},
		{
			name:   "ignored IIFE callback argument",
			source: `func walk(f func()) { func(cb func()) {}(f) }`,
			want:   0,
		},
		{
			name:   "invoked IIFE callback argument",
			source: `func walk(f func()) { func(cb func()) { cb() }(f) }`,
			want:   1,
		},
		{
			name: "safe named conversion",
			source: `type callback func()
func ignore(callback) {}
func walk(f func()) { ignore(callback(f)) }`,
			want: 0,
		},
		{
			name: "stable named alias overwrites callback",
			source: `func setter(dst *func(), cb func()) { cb = func() {}; *dst = cb }
func walk(f func()) { var dst func(); set := setter; set(&dst, f) }`,
			want: 0,
		},
		{
			name: "stable named alias invokes callback",
			source: `func invoke(cb func()) { cb() }
func walk(f func()) { call := invoke; call(f) }`,
			want: 1,
		},
		{
			name: "rebound alias remains unknown",
			source: `func ignore(func()) {}; func invoke(cb func()) { cb() }
func walk(f func(), b bool) { call := ignore; if b { call = invoke }; call(f) }`,
			unknown: true,
		},
		{
			name: "aliased method expression preserves receiver offset",
			source: `type runner struct{}; func (runner) invoke(cb func()) { cb() }
func walk(f func()) { call := runner.invoke; call(runner{}, f) }`,
			want: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pass := ps6080CallbackGrowthPass(t, "package probe\n"+test.source)
			graph := ps6080BuildCallbackGraph(pass)
			function := pass.Pkg.Scope().Lookup("walk").(*types.Func)
			node := graph.nodes[ps6080CallbackNodeKey{function: function, parameter: 0}]
			if node == nil {
				t.Fatal("callback parameter node missing")
			}
			if len(node.reachable) != test.want || node.unknown != test.unknown {
				t.Fatalf("reachable=%d unknown=%v, want %d/%v", len(node.reachable), node.unknown, test.want, test.unknown)
			}
		})
	}
}
