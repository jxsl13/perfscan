package benchmarks

import (
	"maps"
	"slices"
	"testing"
)

var mapPS5045 = func() map[int]int {
	m := map[int]int{}
	for i := 0; i < 1024; i++ {
		m[i] = i
	}
	return m
}()
var sinkPS5045 int

// BenchmarkPS5045Before is len(slices.Collect(maps.Keys(m))): a full slice of
// every key is allocated just to read its length.
func BenchmarkPS5045Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS5045 = len(slices.Collect(maps.Keys(mapPS5045)))
	}
}

// BenchmarkPS5045After is len(m): O(1), no allocation.
func BenchmarkPS5045After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS5045 = len(mapPS5045)
	}
}
