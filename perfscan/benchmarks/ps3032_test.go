package benchmarks

import (
	"slices"
	"sort"
	"testing"
)

// PS3032 — sort.IsSorted over the sort.IntSlice adapter (an interface Len
// plus a Less dispatch per adjacent pair, plus boxing the adapter into
// sort.Interface) vs the generic slices.IsSorted. A sorted 10k-element
// []int is scanned each iteration — the worst case, a full pass over every
// adjacent pair — so the delta is the interface dispatch per comparison
// that the monomorphized generic does not pay. Unlike PS3008's helper
// spelling, the adapter spelling never became a slices.IsSorted wrapper:
// sort.IsSorted takes a sort.Interface on every toolchain. The verdict is
// identical (no adjacent pair descending under <), so both sides compute
// the same bool.
var ps3032Ints = func() []int {
	out := make([]int, 10000)
	for i := range out {
		out[i] = i / 2 // sorted, tie-heavy: ties are not violations
	}
	return out
}()

var ps3032Sink bool

func BenchmarkPS3032_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3032Sink = sort.IsSorted(sort.IntSlice(ps3032Ints))
	}
}

func BenchmarkPS3032_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3032Sink = slices.IsSorted(ps3032Ints)
	}
}
