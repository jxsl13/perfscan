package benchmarks

import (
	"slices"
	"testing"
)

// PS3026 — slices.SortedFunc with a hand-rolled three-way comparator vs
// slices.Sorted. Both collect the same 4096-element scattered iter.Seq[int]
// (identical slices.Collect growth allocations) and sort it; SortedFunc pays
// an indirect comparator call (whose body performs up to TWO relational
// comparisons to synthesize the -1/0/1 sign) on every one of the O(n log n)
// comparisons, which the monomorphized slices.Sort inlines to a single '<'.
// The delta is pure comparison overhead — the same win PS3013 measures for
// the eager SortFunc form and PS3012 for the cmp.Compare spelling.
var ps3026Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff // scattered, distinct-ish
	}
	return a
}()

var ps3026Sink []int

func BenchmarkPS3026_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3026Sink = slices.SortedFunc(slices.Values(ps3026Data), func(a, b int) int {
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

func BenchmarkPS3026_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3026Sink = slices.Sorted(slices.Values(ps3026Data))
	}
}
