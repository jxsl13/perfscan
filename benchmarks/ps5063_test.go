package benchmarks

import (
	"slices"
	"testing"
)

func ps5063Pair() ([]int, []int) {
	a := make([]int, 256)
	for i := range a {
		a[i] = i
	}
	b := slices.Clone(a)
	b[255] = 999 // differ only in the last element
	return a, b
}

var ps5063R bool

// BenchmarkPS5063Before is slices.Compare(a, b) == 0: a full three-way ordering
// scan whose result is only tested for equality.
func BenchmarkPS5063Before(b *testing.B) {
	x, y := ps5063Pair()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5063R = slices.Compare(x, y) == 0
	}
}

// BenchmarkPS5063After is slices.Equal(a, b): equality only, no ordering.
func BenchmarkPS5063After(b *testing.B) {
	x, y := ps5063Pair()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5063R = slices.Equal(x, y)
	}
}
