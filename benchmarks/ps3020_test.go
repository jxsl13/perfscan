package benchmarks

import (
	"cmp"
	"slices"
	"testing"
)

// PS3020 — slices.MaxFunc with a SWAPPED cmp.Compare(b, a) comparator (the
// natural minimum spelled the slow way) vs slices.Min. Both walk a shuffled
// 4096-element []int once and keep the smallest; MaxFunc pays an indirect
// comparator call (plus the cmp.Compare hop inside the literal) on every
// element, which the monomorphized slices.Min (via the builtin min) inlines.
// 0 allocs either way; the delta is pure comparison overhead.
var ps3020Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff // scattered, distinct-ish
	}
	return a
}()

var ps3020Sink int

func BenchmarkPS3020_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3020Sink = slices.MaxFunc(ps3020Data, func(a, b int) int { return cmp.Compare(b, a) })
	}
}

func BenchmarkPS3020_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3020Sink = slices.Min(ps3020Data)
	}
}
