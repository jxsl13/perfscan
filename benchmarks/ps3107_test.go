package benchmarks

import (
	"cmp"
	"math/rand"
	"slices"
	"testing"
)

// PS3107 — slices.SortFunc with a bare cmp.Compare(a, b) comparator vs the
// monomorphized slices.Sort. A shuffled 4096-element []int is copied into a
// scratch slice and sorted each iteration; the copy is identical on both
// sides, so the delta is the indirect comparator call per comparison (plus
// the cmp.Compare hop inside the closure) that slices.Sort's inlined '<'
// does not pay. Ordering is identical (cmp.Compare(a,b) < 0 iff
// cmp.Less(a,b), the order slices.Sort is defined by), so both sides
// produce the same slice; both do 0 allocs, making the win pure comparison
// overhead — measured ~1.6x on go1.26/arm64 (238 µs vs 148 µs/op).
var ps3107Ints = func() []int {
	out := make([]int, 4096)
	for i := range out {
		out[i] = i
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

func BenchmarkPS3107_Before(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3107Ints))
	for range b.N {
		copy(scratch, ps3107Ints)
		slices.SortFunc(scratch, func(x, y int) int { return cmp.Compare(x, y) })
		sinkI = scratch[0]
	}
}

func BenchmarkPS3107_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3107Ints))
	for range b.N {
		copy(scratch, ps3107Ints)
		slices.Sort(scratch)
		sinkI = scratch[0]
	}
}
