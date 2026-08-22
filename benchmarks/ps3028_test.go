package benchmarks

import (
	"slices"
	"testing"
)

// PS3028 — slices.BinarySearchFunc with a hand-rolled three-way comparator
// (a<b/a>b/-1/1/0) vs slices.BinarySearch. Both make the identical log n
// probes over the same sorted []int and return the identical (index, found);
// BinarySearchFunc pays an indirect comparator call plus up to two relational
// comparisons inside the hand-rolled chain on each probe, which the
// monomorphized slices.BinarySearch replaces with one inlined comparison.
// Targets cycle across the whole range so every search runs a full log n
// probes. 0 allocs either way.
var ps3028Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = i * 2 // sorted, distinct
	}
	return a
}()

var (
	ps3028Idx   int
	ps3028Found bool
)

func BenchmarkPS3028_Before(b *testing.B) {
	b.ReportAllocs()
	for n := range b.N {
		ps3028Idx, ps3028Found = slices.BinarySearchFunc(ps3028Data, (n%4096)*2, func(a, b int) int {
			if a < b {
				return -1
			}
			if a > b {
				return 1
			}
			return 0
		})
	}
}

func BenchmarkPS3028_After(b *testing.B) {
	b.ReportAllocs()
	for n := range b.N {
		ps3028Idx, ps3028Found = slices.BinarySearch(ps3028Data, (n%4096)*2)
	}
}
