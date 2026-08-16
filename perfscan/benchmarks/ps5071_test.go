package benchmarks

import (
	"strconv"
	"testing"
)

// PS5071 — strconv.Itoa(x) == "200" vs x == 200. The Before side heap-
// allocates the decimal string of x and runs a base-10 formatting pass just
// to compare it against the constant; the After side is one integer compare.
var (
	ps5071Status = 404
	ps5071Sink   bool
)

func BenchmarkPS5071_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5071Sink = strconv.Itoa(ps5071Status) == "200"
	}
}

func BenchmarkPS5071_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5071Sink = ps5071Status == 200
	}
}
