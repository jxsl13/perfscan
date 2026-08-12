package benchmarks

import (
	"math/rand"
	"slices"
	"sort"
	"testing"
)

// PS3105 — sort.Sort(sort.IntSlice(x)) through the sort.Interface adapter
// vs the generic slices.Sort. A shuffled 10k-element []int is copied into
// a scratch slice and sorted each iteration; the copy is identical on both
// sides, so the delta is the adapter's interface dispatch per
// comparison/swap that the compile-time-specialized pdqsort does not pay
// (plus the one allocation boxing the adapter into sort.Interface).
// Ordering is identical (ascending by <), so both sides produce the same
// slice. Unlike sort.Ints (PS3104, parity since go1.22), sort.Sort never
// became a wrapper over slices.Sort, so the dispatch cost is real on
// modern toolchains: measured ~1.7x on go1.26 (678 µs vs 389 µs/op).
var ps3105Ints = func() []int {
	out := make([]int, 10000)
	for i := range out {
		out[i] = i
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

func BenchmarkPS3105_Before(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3105Ints))
	for range b.N {
		copy(scratch, ps3105Ints)
		//lint:ignore S1032 this benchmark deliberately measures the sort.Sort(sort.IntSlice(x)) anti-pattern that PS3105 fixes
		sort.Sort(sort.IntSlice(scratch))
		sinkI = scratch[0]
	}
}

func BenchmarkPS3105_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3105Ints))
	for range b.N {
		copy(scratch, ps3105Ints)
		slices.Sort(scratch)
		sinkI = scratch[0]
	}
}
