package benchmarks

import (
	"slices"
	"sort"
	"testing"
)

// PS3008 — sort.IntsAreSorted through the sort.Interface indirection vs the
// generic slices.IsSorted. A sorted 10k-element []int is scanned each
// iteration — the worst case, a full pass over every adjacent pair — so the
// delta is the interface dispatch per comparison that the monomorphized
// generic does not pay. The verdict is identical (no adjacent pair
// descending under <), so both sides compute the same bool.
var ps3008Ints = func() []int {
	out := make([]int, 10000)
	for i := range out {
		out[i] = i / 2 // sorted, tie-heavy: ties are not violations
	}
	return out
}()

var ps3008Sink bool

func BenchmarkPS3008_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3008Sink = sort.IntsAreSorted(ps3008Ints)
	}
}

func BenchmarkPS3008_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3008Sink = slices.IsSorted(ps3008Ints)
	}
}
