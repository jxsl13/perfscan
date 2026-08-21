package benchmarks

import (
	"cmp"
	"slices"
	"testing"
)

// PS3012 — slices.SortedFunc with a bare cmp.Compare comparator vs
// slices.Sorted. Both collect the same 4096-element scattered iter.Seq[int]
// (identical slices.Collect growth allocations) and sort it; SortedFunc pays
// an indirect comparator call (plus the cmp.Compare hop inside the literal)
// on every one of the O(n log n) comparisons, which the monomorphized
// slices.Sort inlines. The delta is pure comparison overhead — the same win
// PS3107 measures for the eager SortFunc form.
var ps3012Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff // scattered, distinct-ish
	}
	return a
}()

var ps3012Sink []int

func BenchmarkPS3012_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3012Sink = slices.SortedFunc(slices.Values(ps3012Data), func(a, b int) int { return cmp.Compare(a, b) })
	}
}

func BenchmarkPS3012_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3012Sink = slices.Sorted(slices.Values(ps3012Data))
	}
}
