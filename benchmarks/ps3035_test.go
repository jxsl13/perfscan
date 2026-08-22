package benchmarks

import (
	"slices"
	"testing"
)

// PS3035 — slices.MaxFunc with a SWAPPED hand-rolled three-way comparator
// (a>b -> -1, a<b -> 1: the reversed order, so the call computes the MINIMUM)
// vs slices.Min. Both walk a scattered 4096-element []int once and keep the
// smallest; MaxFunc pays an indirect comparator call plus up to two
// relational comparisons (a>b, then a<b) on every element, which the
// monomorphized slices.Min (via the builtin min) folds to a single inlined
// comparison. 0 allocs either way; the delta is pure comparison overhead
// (on gc 1.26 the fresh literal devirtualizes to ~parity — the removed
// indirection is the win on toolchains without closure devirtualization,
// the same class as PS3020/PS3022).
var ps3035Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff // scattered, distinct-ish
	}
	return a
}()

var ps3035Sink int

func BenchmarkPS3035_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3035Sink = slices.MaxFunc(ps3035Data, func(a, b int) int {
			if a > b {
				return -1
			}
			if a < b {
				return 1
			}
			return 0
		})
	}
}

func BenchmarkPS3035_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3035Sink = slices.Min(ps3035Data)
	}
}
