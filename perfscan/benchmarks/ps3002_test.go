package benchmarks

import (
	"cmp"
	"math/rand"
	"slices"
	"sort"
	"testing"
)

// PS3002 — sort.Slice performs reflect-based element swaps AND calls its
// comparator closure through an interface: two indirections per operation that
// the compile-time-specialized slices.Sort / slices.SortFunc avoid. Both fix
// forms the check emits are measured, each sorting a shuffled 10k copy (the copy
// is identical on both sides, so the delta is the reflection + dispatch
// overhead). Ordering is identical (ascending by <), so both sides agree.

type ps3002Item struct {
	key int
	pad [3]int // a larger element makes the reflect-based swaps cost more
}

var ps3002Items = func() []ps3002Item {
	out := make([]ps3002Item, 10000)
	rng := rand.New(rand.NewSource(1))
	for i := range out {
		out[i] = ps3002Item{key: rng.Intn(1 << 20), pad: [3]int{i, 0, 0}}
	}
	return out
}()

var ps3002Ints = func() []int {
	out := make([]int, 10000)
	for i := range out {
		out[i] = i
	}
	rand.New(rand.NewSource(2)).Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

// Field compare: sort.Slice(func(i,j){ xs[i].key < xs[j].key }) -> slices.SortFunc.
func BenchmarkPS3002Field_Before(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]ps3002Item, len(ps3002Items))
	for range b.N {
		copy(scratch, ps3002Items)
		sort.Slice(scratch, func(i, j int) bool { return scratch[i].key < scratch[j].key })
		sinkI = scratch[0].key + scratch[0].pad[0]
	}
}

func BenchmarkPS3002Field_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]ps3002Item, len(ps3002Items))
	for range b.N {
		copy(scratch, ps3002Items)
		slices.SortFunc(scratch, func(a, b ps3002Item) int { return cmp.Compare(a.key, b.key) })
		sinkI = scratch[0].key + scratch[0].pad[0]
	}
}

// Whole-element compare: sort.Slice(func(i,j){ xs[i] < xs[j] }) -> slices.Sort.
func BenchmarkPS3002Whole_Before(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3002Ints))
	for range b.N {
		copy(scratch, ps3002Ints)
		sort.Slice(scratch, func(i, j int) bool { return scratch[i] < scratch[j] })
		sinkI = scratch[0]
	}
}

func BenchmarkPS3002Whole_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]int, len(ps3002Ints))
	for range b.N {
		copy(scratch, ps3002Ints)
		slices.Sort(scratch)
		sinkI = scratch[0]
	}
}
