package benchmarks

import (
	"math/rand"
	"slices"
	"testing"
)

// PS3013 — slices.SortFunc with a hand-rolled three-way if-chain comparator
// vs the monomorphized slices.Sort. A shuffled 4096-element []int is copied
// into a scratch slice and sorted each iteration; the copy is identical on
// both sides, so the delta is the indirect comparator call plus the up-to-two
// relational comparisons per element pair that slices.Sort's single inlined
// '<' does not pay. Ordering is identical (the chain's sign equals
// cmp.Compare's, and cmp.Compare(a,b) < 0 iff cmp.Less(a,b), the order
// slices.Sort is defined by), so both sides produce the same slice; both do
// 0 allocs, making the win pure comparison overhead — measured ~1.6x on
// go1.26/arm64, the same character as PS3107's cmp.Compare spelling.
var ps3013Ints = func() []int {
	out := make([]int, 4096)
	for i := range out {
		out[i] = i
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

func BenchmarkPS3013_Before(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3013Ints))
	for range b.N {
		copy(scratch, ps3013Ints)
		slices.SortFunc(scratch, func(x, y int) int {
			if x < y {
				return -1
			}
			if x > y {
				return 1
			}
			return 0
		})
		sinkI = scratch[0]
	}
}

func BenchmarkPS3013_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3013Ints))
	for range b.N {
		copy(scratch, ps3013Ints)
		slices.Sort(scratch)
		sinkI = scratch[0]
	}
}
