package benchmarks

import (
	"cmp"
	"slices"
	"testing"
)

// PS3010 — slices.IsSortedFunc with a bare cmp.Compare comparator vs
// slices.IsSorted. Both scan a sorted 4096-element []int once (sorted input
// forces the full O(n) pass — no early return); IsSortedFunc pays an indirect
// comparator call (plus the cmp.Compare hop inside the literal) on every
// adjacent pair, which the monomorphized slices.IsSorted inlines. 0 allocs
// either way; the delta is pure comparison overhead.
var ps3010Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = i / 2 // ascending with tie plateaus
	}
	return a
}()

var ps3010Sink bool

func BenchmarkPS3010_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3010Sink = slices.IsSortedFunc(ps3010Data, func(a, b int) int { return cmp.Compare(a, b) })
	}
}

func BenchmarkPS3010_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3010Sink = slices.IsSorted(ps3010Data)
	}
}
