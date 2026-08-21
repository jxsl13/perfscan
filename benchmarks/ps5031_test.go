package benchmarks

import (
	"strings"
	"testing"
)

// PS5031 — strings.LastIndex compared against -1 for membership vs the
// strings.Contains form the fix emits. For a multi-byte needle LastIndex
// always runs its pure-Go BACKWARD Rabin-Karp (reverse rolling hash, no
// assembly path, no early exit possible — it must find the LAST
// occurrence), while Contains is Index >= 0: the bytealg forward scan
// (IndexByte-accelerated/SIMD) that returns at the FIRST match. The
// haystack is ~11.3 KB of log lines with the probed key ONLY in the
// header line, so the sole occurrence sits near the front: the backward
// scan hashes essentially the whole haystack before reaching it, the
// forward scan short-circuits almost immediately. The absent-needle pair
// bounds the win — both sides scan everything, SIMD forward vs scalar
// reverse hash. Zero allocations on every side; the win is pure scan
// cost and grows with the haystack.
var (
	ps5031Haystack = "request-id=deadbeef region=eu-west-1 trace=42\n" +
		strings.Repeat("service=checkout shard=07 cost=42 status=ok\n", 256)
	ps5031Present = "eu-west-1" // sole match at byte offset 27
	ps5031Absent  = "eu-EAST-9"
)

var ps5031Sink bool

func BenchmarkPS5031Present_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5031Sink = strings.LastIndex(ps5031Haystack, ps5031Present) != -1
	}
}

func BenchmarkPS5031Present_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5031Sink = strings.Contains(ps5031Haystack, ps5031Present)
	}
}

func BenchmarkPS5031Absent_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5031Sink = strings.LastIndex(ps5031Haystack, ps5031Absent) != -1
	}
}

func BenchmarkPS5031Absent_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5031Sink = strings.Contains(ps5031Haystack, ps5031Absent)
	}
}
