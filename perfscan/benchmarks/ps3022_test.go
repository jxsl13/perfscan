package benchmarks

import (
	"slices"
	"testing"
)

// PS3022 — slices.MaxFunc with a hand-rolled three-way comparator vs
// slices.Max. Both walk a scattered 4096-element []int once and keep the
// greatest; MaxFunc pays an indirect comparator call plus up to two
// relational comparisons (a<b, then a>b) on every element, which the
// monomorphized slices.Max (via the builtin max) folds to a single inlined
// comparison. 0 allocs either way; the delta is pure comparison overhead
// (on gc 1.26 the fresh literal devirtualizes to ~parity — the removed
// indirection is the win on toolchains without closure devirtualization,
// the same class as PS3111).
var ps3022Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff // scattered, distinct-ish
	}
	return a
}()

var ps3022Sink int

func BenchmarkPS3022_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3022Sink = slices.MaxFunc(ps3022Data, func(a, b int) int {
			if a < b {
				return -1
			}
			if a > b {
				return 1
			}
			return 0
		})
	}
}

func BenchmarkPS3022_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3022Sink = slices.Max(ps3022Data)
	}
}
