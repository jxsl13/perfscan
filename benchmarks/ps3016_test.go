package benchmarks

import (
	"cmp"
	"slices"
	"testing"
)

// PS3016 — slices.SortedStableFunc with a bare cmp.Compare comparator vs
// slices.Sorted. Both collect the same 4096-element scattered iter.Seq[int]
// (identical slices.Collect growth allocations) and sort it; the before side
// pays TWICE — an indirect comparator call (plus the cmp.Compare hop inside
// the literal) on every one of the O(n log n) comparisons, and the STABLE
// insertion-run/symMerge algorithm's extra moves — both of which the
// monomorphized unstable slices.Sort inside slices.Sorted drops. The delta
// stacks the PS3012 (comparator indirection) and PS3006 (stability overhead)
// wins.
var ps3016Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff // scattered, distinct-ish
	}
	return a
}()

var ps3016Sink []int

func BenchmarkPS3016_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3016Sink = slices.SortedStableFunc(slices.Values(ps3016Data), func(a, b int) int { return cmp.Compare(a, b) })
	}
}

func BenchmarkPS3016_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3016Sink = slices.Sorted(slices.Values(ps3016Data))
	}
}
