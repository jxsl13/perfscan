package benchmarks

import (
	"cmp"
	"slices"
	"testing"
)

// PS3111 — slices.MaxFunc with a bare cmp.Compare comparator vs slices.Max. Both
// walk a shuffled 4096-element []int once and keep the greatest; MaxFunc pays an
// indirect comparator call (plus the cmp.Compare hop inside the literal) on every
// element, which the monomorphized slices.Max (via the builtin max) inlines. 0
// allocs either way; the delta is pure comparison overhead.
var ps3111Data = func() []int {
	a := make([]int, 4096)
	for i := range a {
		a[i] = (i*2654435761 + 7) & 0x7fffffff // scattered, distinct-ish
	}
	return a
}()

var ps3111Sink int

func BenchmarkPS3111_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3111Sink = slices.MaxFunc(ps3111Data, func(a, b int) int { return cmp.Compare(a, b) })
	}
}

func BenchmarkPS3111_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3111Sink = slices.Max(ps3111Data)
	}
}
