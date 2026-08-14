package benchmarks

import (
	"slices"
	"testing"
)

// PS3108 — slices.CompactFunc with a bare x == y equality closure vs
// slices.Compact. A 4096-element []int with every element DISTINCT is the worst
// case (no run is ever collapsed, so every adjacent pair goes through the
// equality) and is idempotent — Compact removes nothing and returns the slice
// unchanged, so repeated calls on the same backing array are well-defined. Both
// sides compare adjacent elements with the identical ==, so the result is the
// same on every input; both do 0 allocs. Expected PARITY on current gc (it
// inlines slices.CompactFunc and devirtualizes the literal closure, so both
// compile to the same direct-comparison loop); toolchains without closure
// devirtualization drop a real per-pair indirect call.
var ps3108Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = i // all distinct: worst case, nothing collapsed, idempotent
	}
	return a
}()

var ps3108Sink []int

func BenchmarkPS3108_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3108Sink = slices.CompactFunc(ps3108Data, func(a, b int) bool { return a == b })
	}
}

func BenchmarkPS3108_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3108Sink = slices.Compact(ps3108Data)
	}
}
