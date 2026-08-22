package benchmarks

import (
	"slices"
	"sort"
	"testing"
)

// PS3024 — sort.SliceIsSorted with the whole-element ascending closure
// (interface boxing + reflectlite length read + an indirect closure call
// per adjacent pair) vs the generic slices.IsSorted. A sorted 10k-element
// []int is scanned each iteration — the worst case, a full pass over every
// adjacent pair — so the delta is the per-comparison closure indirection
// the monomorphized generic does not pay. The verdict is identical (no
// adjacent pair descending under <), so both sides compute the same bool.
var ps3024Ints = func() []int {
	out := make([]int, 10000)
	for i := range out {
		out[i] = i / 2 // sorted, tie-heavy: ties are not violations
	}
	return out
}()

var ps3024Sink bool

func BenchmarkPS3024_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		xs := ps3024Ints
		ps3024Sink = sort.SliceIsSorted(xs, func(i, j int) bool { return xs[i] < xs[j] })
	}
}

func BenchmarkPS3024_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		xs := ps3024Ints
		ps3024Sink = slices.IsSorted(xs)
	}
}
