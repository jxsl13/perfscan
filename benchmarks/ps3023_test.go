package benchmarks

import (
	"cmp"
	"slices"
	"testing"
)

// PS3023 — slices.CompareFunc with a bare cmp.Compare comparator vs
// slices.Compare. Both compare two 4096-element []int slices that agree on
// every element but the last — a full-length lexicographic scan — but
// CompareFunc pays an indirect comparator call (plus the cmp.Compare hop
// inside the literal) on every element pair, which the monomorphized
// slices.Compare inlines. 0 allocs either way; the delta is pure comparison
// overhead.
var ps3023A, ps3023B = func() ([]int, []int) {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff
	}
	b := slices.Clone(a)
	b[len(b)-1]-- // equal prefix, divergence at the very last pair
	return a, b
}()

var ps3023Sink int

func BenchmarkPS3023_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3023Sink = slices.CompareFunc(ps3023A, ps3023B, func(x, y int) int { return cmp.Compare(x, y) })
	}
}

func BenchmarkPS3023_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3023Sink = slices.Compare(ps3023A, ps3023B)
	}
}
