package benchmarks

import (
	"strconv"
	"testing"
)

var (
	ps5051A int64 = 1234567
	ps5051B int64 = 1234568
	ps5051R bool
)

// BenchmarkPS5051Before is the strconv.FormatInt(a, B) == strconv.FormatInt(b, B)
// form the check flags: two string allocations and two base-B formatting passes.
func BenchmarkPS5051Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5051R = strconv.FormatInt(ps5051A, 16) == strconv.FormatInt(ps5051B, 16)
	}
}

// BenchmarkPS5051After is the a == b rewrite: one integer compare, no allocation.
func BenchmarkPS5051After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5051R = ps5051A == ps5051B
	}
}
