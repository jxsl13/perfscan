package benchmarks

import (
	"strconv"
	"testing"
)

var sinkPS5048 bool

// BenchmarkPS5048Before is strconv.Itoa(a) == strconv.Itoa(b): two decimal
// strings allocated and formatted just to compare.
func BenchmarkPS5048Before(b *testing.B) {
	x, y := 1234567, 1234568
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS5048 = strconv.Itoa(x) == strconv.Itoa(y)
	}
}

// BenchmarkPS5048After is a == b: one integer comparison, no allocation.
func BenchmarkPS5048After(b *testing.B) {
	x, y := 1234567, 1234568
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS5048 = x == y
	}
}
