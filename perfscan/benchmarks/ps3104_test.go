package benchmarks

import (
	"math/rand"
	"slices"
	"sort"
	"testing"
)

// PS3104 — sort.Ints through the sort.Interface indirection vs the generic
// slices.Sort. A shuffled 10k-element []int is copied into a scratch slice
// and sorted each iteration; the copy is identical on both sides, so the
// delta is the interface dispatch per comparison/swap that the
// compile-time-specialized pdqsort does not pay. Ordering is identical
// (ascending by <), so both sides produce the same slice.
var ps3104Ints = func() []int {
	out := make([]int, 10000)
	for i := range out {
		out[i] = i
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

func BenchmarkPS3104_Before(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3104Ints))
	for range b.N {
		copy(scratch, ps3104Ints)
		sort.Ints(scratch)
		sinkI = scratch[0]
	}
}

func BenchmarkPS3104_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3104Ints))
	for range b.N {
		copy(scratch, ps3104Ints)
		slices.Sort(scratch)
		sinkI = scratch[0]
	}
}
