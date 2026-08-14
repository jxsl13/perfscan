package benchmarks

import (
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// PS3009 — slices.SortFunc / slices.SortStableFunc with a bare
// strings.Compare(a, b) comparator vs the monomorphized slices.Sort. A
// shuffled tie-heavy 4096-element []string is copied into a scratch slice
// and sorted each iteration; the copy is identical on all sides, so the
// delta is the indirect comparator hop per comparison that slices.Sort's
// inlined '<' does not pay — and for the stable spelling additionally the
// insertion-run/symMerge machinery pdqsort skips. The delta is smaller
// than the []int siblings' (PS3107/PS3006) because the string comparison
// itself is a runtime cmpstring call on BOTH sides. Ordering is identical
// (strings.Compare(a,b) < 0 iff a < b byte-lexicographically, the order
// slices.Sort is defined by) and byte-equal ties are interchangeable, so
// all sides produce the same slice; all do 0 allocs — measured ~1.15x
// (unstable) and ~2.0x (stable) on go1.26/arm64 (435 µs and 750 µs vs
// 377 µs/op).
var ps3009Words = func() []string {
	out := make([]string, 4096)
	for i := range out {
		out[i] = "w-" + strconv.Itoa(i%1024) // duplicates -> real ties
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}()

func BenchmarkPS3009_Before(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]string, len(ps3009Words))
	for range b.N {
		copy(scratch, ps3009Words)
		slices.SortFunc(scratch, func(x, y string) int { return strings.Compare(x, y) })
		sinkS = scratch[0]
	}
}

func BenchmarkPS3009_BeforeStable(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]string, len(ps3009Words))
	for range b.N {
		copy(scratch, ps3009Words)
		slices.SortStableFunc(scratch, func(x, y string) int { return strings.Compare(x, y) })
		sinkS = scratch[0]
	}
}

func BenchmarkPS3009_After(b *testing.B) {
	b.ReportAllocs()
	scratch := make([]string, len(ps3009Words))
	for range b.N {
		copy(scratch, ps3009Words)
		slices.Sort(scratch)
		sinkS = scratch[0]
	}
}
