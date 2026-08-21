package benchmarks

import (
	"slices"
	"sort"
	"testing"
)

func ps3037Ints() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = i * 2
	}
	return a
}

var ps3037Sink int

// BenchmarkPS3037Before is the sort.SearchInts(a, x) form the check flags: a
// binary search driven through a per-probe closure.
func BenchmarkPS3037Before(b *testing.B) {
	a := ps3037Ints()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps3037Sink = sort.SearchInts(a, i%8192)
	}
}

// BenchmarkPS3037After is the slices.BinarySearch(a, x) rewrite: the same search
// with no per-probe closure.
func BenchmarkPS3037After(b *testing.B) {
	a := ps3037Ints()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps3037Sink, _ = slices.BinarySearch(a, i%8192)
	}
}
