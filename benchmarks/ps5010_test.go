package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS5010 — string(bytes.ToUpper([]byte(s))) vs strings.ToUpper(s). The
// Before round-trips s through []byte: []byte(s) copies s in,
// bytes.ToUpper allocates and fills a fresh result slice, and
// string(...) copies that result out (on current gc, escape analysis
// stack-allocates the []byte(s) conversion in this exact shape, so the
// measured Before cost is two heap allocations; in shapes it cannot
// prove, all three hit the heap). The After reads straight off the
// string header and allocates at most the single result string. Two
// inputs bracket the win: a mixed-case line containing ß (the mapping
// must allocate — the After saves the round-trip copy) and an
// all-ASCII already-uppercase line (the After's fast path returns s
// itself: zero allocations against the Before's unconditional two).
var (
	ps5010Mixed = "mixed-Case straße payload with Fifty-five Bytes total!"
	ps5010Upper = "AN ALREADY-UPPERCASE ASCII LINE THAT NEEDS NO MAPPING!"
)

func BenchmarkPS5010_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.ToUpper([]byte(ps5010Mixed)))
	}
}

func BenchmarkPS5010_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.ToUpper(ps5010Mixed)
	}
}

func BenchmarkPS5010_NoChange_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.ToUpper([]byte(ps5010Upper)))
	}
}

func BenchmarkPS5010_NoChange_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.ToUpper(ps5010Upper)
	}
}
