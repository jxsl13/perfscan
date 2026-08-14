package benchmarks

import (
	"slices"
	"testing"
)

// PS3110 — slices.IndexFunc with a bare x == target closure vs slices.Index.
// A 4096-element []int searched for a target that is ABSENT (the worst case: no
// early exit, every element goes through the equality). Both scan left to right
// with the identical ==, so the returned index is the same on every input; 0
// allocs either way. Expected parity on current gc (it inlines slices.IndexFunc
// and devirtualizes the literal closure, so both become the same direct scan);
// toolchains without closure devirtualization drop a real per-element indirect
// call.
var ps3110Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = i
	}
	return a
}()

var ps3110Sink int

func BenchmarkPS3110_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3110Sink = slices.IndexFunc(ps3110Data, func(x int) bool { return x == -1 })
	}
}

func BenchmarkPS3110_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3110Sink = slices.Index(ps3110Data, -1)
	}
}
