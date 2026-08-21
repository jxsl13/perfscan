package benchmarks

import (
	"slices"
	"testing"
)

// PS3027 — slices.CompareFunc with a hand-rolled three-way comparator
// (exact -1/+1/0) vs slices.Compare. Both compare two 4096-element []int
// slices that agree on every element but the last — a full-length
// lexicographic scan — but CompareFunc pays an indirect comparator call plus
// up to two relational comparisons (a<b, then a>b) on every element pair,
// which the monomorphized slices.Compare inlines via cmp.Compare. 0 allocs
// either way; the delta is pure comparison overhead (on gc 1.26 the fresh
// literal devirtualizes to ~parity — the removed indirection is the win on
// toolchains without closure devirtualization, the same class as PS3023 and
// the PS3013/PS3022 siblings).
var ps3027A, ps3027B = func() ([]int, []int) {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff
	}
	b := slices.Clone(a)
	b[len(b)-1]-- // equal prefix, divergence at the very last pair
	return a, b
}()

var ps3027Sink int

func BenchmarkPS3027_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3027Sink = slices.CompareFunc(ps3027A, ps3027B, func(x, y int) int {
			if x < y {
				return -1
			}
			if x > y {
				return 1
			}
			return 0
		})
	}
}

func BenchmarkPS3027_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3027Sink = slices.Compare(ps3027A, ps3027B)
	}
}
