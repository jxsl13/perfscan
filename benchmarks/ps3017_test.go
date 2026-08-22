package benchmarks

import (
	"math/rand"
	"slices"
	"testing"
)

// PS3017 — slices.SortStableFunc with a hand-rolled three-way if-chain
// comparator vs the monomorphized slices.Sort. A shuffled 4096-element []int
// is copied into a scratch slice and sorted each iteration; the copy is
// identical on both sides, so the delta stacks the family's two savings: the
// indirect comparator call plus the up-to-two relational comparisons per
// element pair that slices.Sort's single inlined '<' does not pay (PS3013's
// ~1.6x share), and the stable insertion-run/symMerge machinery that the
// unstable pdqsort skips entirely (PS3006's share). Ordering is identical
// (the chain's sign equals cmp.Compare's, cmp.Compare(a,b) < 0 iff
// cmp.Less(a,b), and equal ints are bitwise-identical so stability is
// unobservable); both sides do 0 allocs — measured ~3.7x on go1.26/arm64.
var ps3017Ints = func() []int {
	out := make([]int, 4096)
	for i := range out {
		out[i] = i
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

func BenchmarkPS3017_Before(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3017Ints))
	for range b.N {
		copy(scratch, ps3017Ints)
		slices.SortStableFunc(scratch, func(x, y int) int {
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

func BenchmarkPS3017_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3017Ints))
	for range b.N {
		copy(scratch, ps3017Ints)
		slices.Sort(scratch)
		sinkI = scratch[0]
	}
}
