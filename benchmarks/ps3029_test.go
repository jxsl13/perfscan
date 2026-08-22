package benchmarks

import (
	"slices"
	"testing"
)

// PS3029 — slices.SortedStableFunc with a hand-rolled three-way comparator
// vs slices.Sorted. Both collect the same 4096-element scattered
// iter.Seq[int] (identical slices.Collect growth allocations) and sort it;
// the before side pays THREE ways — an indirect comparator call on every one
// of the O(n log n) comparisons, up to TWO relational comparisons inside the
// ladder to synthesize a sign the sort consumes as a bool, and the STABLE
// insertion-run/symMerge algorithm's extra moves — all of which the
// monomorphized unstable slices.Sort inside slices.Sorted drops. The delta
// stacks the PS3026 (hand-rolled comparator indirection) and PS3016
// (stability overhead) wins.
var ps3029Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff // scattered, distinct-ish
	}
	return a
}()

var ps3029Sink []int

func BenchmarkPS3029_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3029Sink = slices.SortedStableFunc(slices.Values(ps3029Data), func(a, b int) int {
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

func BenchmarkPS3029_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3029Sink = slices.Sorted(slices.Values(ps3029Data))
	}
}
