package benchmarks

import (
	"cmp"
	"math/rand"
	"slices"
	"testing"
)

// PS3006 — slices.SortStableFunc with a bare cmp.Compare(a, b) comparator
// vs the monomorphized slices.Sort. A shuffled 4096-element []int is copied
// into a scratch slice and sorted each iteration; the copy is identical on
// both sides, so the delta stacks the TWO costs the rewrite removes: the
// indirect comparator call per comparison (plus the cmp.Compare hop inside
// the closure) that slices.Sort's inlined '<' does not pay, and the stable
// algorithm's insertion-run/symMerge machinery that the unstable pdqsort
// skips. Ordering is identical (cmp.Compare(a,b) < 0 iff cmp.Less(a,b), the
// order slices.Sort is defined by) and int ties are bitwise identical, so
// both sides produce the same slice; both do 0 allocs.
var ps3006Ints = func() []int {
	out := make([]int, 4096)
	for i := range out {
		out[i] = i
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

func BenchmarkPS3006_Before(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3006Ints))
	for range b.N {
		copy(scratch, ps3006Ints)
		slices.SortStableFunc(scratch, func(x, y int) int { return cmp.Compare(x, y) })
		sinkI = scratch[0]
	}
}

func BenchmarkPS3006_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3006Ints))
	for range b.N {
		copy(scratch, ps3006Ints)
		slices.Sort(scratch)
		sinkI = scratch[0]
	}
}
