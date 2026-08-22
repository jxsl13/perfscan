package benchmarks

import (
	"maps"
	"slices"
	"testing"
)

func ps5052Map() map[int]int {
	m := make(map[int]int, 1024)
	for i := 0; i < 1024; i++ {
		m[i] = i
	}
	return m
}

var ps5052Sink int

// BenchmarkPS5052Before is the len(slices.Sorted(maps.Keys(m))) form the check
// flags: a growing []K allocation plus an O(n log n) sort, thrown away.
func BenchmarkPS5052Before(b *testing.B) {
	m := ps5052Map()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5052Sink = len(slices.Sorted(maps.Keys(m)))
	}
}

// BenchmarkPS5052After is the len(m) rewrite: an O(1) header read.
func BenchmarkPS5052After(b *testing.B) {
	m := ps5052Map()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5052Sink = len(m)
	}
}
