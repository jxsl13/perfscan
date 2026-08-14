package benchmarks

import (
	"cmp"
	"slices"
	"testing"
)

// PS3109 — slices.BinarySearchFunc with a bare cmp.Compare comparator vs
// slices.BinarySearch. Both make the identical log n probes over the same sorted
// []int and return the identical (index, found); BinarySearchFunc pays an
// indirect comparator call (plus the cmp.Compare hop inside the literal) on each
// probe, which the monomorphized slices.BinarySearch inlines. Targets cycle
// across the whole range so every search runs a full log n probes. 0 allocs
// either way.
var ps3109Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = i * 2 // sorted, distinct
	}
	return a
}()

var (
	ps3109Idx   int
	ps3109Found bool
)

func BenchmarkPS3109_Before(b *testing.B) {
	b.ReportAllocs()
	for n := range b.N {
		ps3109Idx, ps3109Found = slices.BinarySearchFunc(ps3109Data, (n%4096)*2, func(a, b int) int { return cmp.Compare(a, b) })
	}
}

func BenchmarkPS3109_After(b *testing.B) {
	b.ReportAllocs()
	for n := range b.N {
		ps3109Idx, ps3109Found = slices.BinarySearch(ps3109Data, (n%4096)*2)
	}
}
