package benchmarks

import (
	"strconv"
	"testing"
)

var (
	ps5053A = "hello world example"
	ps5053B = "hello world example!"
	ps5053R bool
)

// BenchmarkPS5053Before is the strconv.Quote(a) == strconv.Quote(b) form the
// check flags: two quoting passes and two throwaway literals.
func BenchmarkPS5053Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5053R = strconv.Quote(ps5053A) == strconv.Quote(ps5053B)
	}
}

// BenchmarkPS5053After is the a == b rewrite: one direct string comparison.
func BenchmarkPS5053After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5053R = ps5053A == ps5053B
	}
}
