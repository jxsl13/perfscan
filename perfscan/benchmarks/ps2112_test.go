package benchmarks

import (
	"slices"
	"testing"
)

// PS2112 — clipped append chain over an empty base vs slices.Concat. The
// chain allocates for the first spread and then grows (reallocating and
// re-copying the first half) for the second; Concat sums the lengths and
// allocates the result once.
func BenchmarkPS2112_Before(b *testing.B) {
	b.ReportAllocs()
	x, y := words[:n/2], words[n/2:]
	for range b.N {
		sinkSS = append(append([]string(nil), x...), y...)
	}
}

func BenchmarkPS2112_After(b *testing.B) {
	b.ReportAllocs()
	x, y := words[:n/2], words[n/2:]
	for range b.N {
		sinkSS = slices.Concat(x, y)
	}
}
