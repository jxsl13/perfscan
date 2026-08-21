package benchmarks

import (
	"bytes"
	"testing"
)

// PS5036 — bytes.LastIndex compared against -1 for membership vs the
// bytes.Contains form the fix emits. For a multi-byte needle LastIndex
// always runs its pure-Go BACKWARD Rabin-Karp (reverse rolling hash, no
// assembly path, no early exit possible — it must find the LAST
// occurrence), while Contains is Index != -1: the bytealg forward scan
// (IndexByte-accelerated/SIMD) that returns at the FIRST match. The
// haystack is ~11.3 KB of log lines with the probed key ONLY in the
// header line, so the sole occurrence sits near the front: the backward
// scan hashes essentially the whole haystack before reaching it, the
// forward scan short-circuits almost immediately. The absent-needle pair
// bounds the win — both sides scan everything, SIMD forward vs scalar
// reverse hash. Zero allocations on every side; the win is pure scan
// cost and grows with the haystack. The bytes twin of BenchmarkPS5031.
var (
	ps5036Haystack = append([]byte("request-id=deadbeef region=eu-west-1 trace=42\n"),
		bytes.Repeat([]byte("service=checkout shard=07 cost=42 status=ok\n"), 256)...)
	ps5036Present = []byte("eu-west-1") // sole match at byte offset 27
	ps5036Absent  = []byte("eu-EAST-9")
)

var ps5036Sink bool

func BenchmarkPS5036Present_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5036Sink = bytes.LastIndex(ps5036Haystack, ps5036Present) != -1
	}
}

func BenchmarkPS5036Present_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5036Sink = bytes.Contains(ps5036Haystack, ps5036Present)
	}
}

func BenchmarkPS5036Absent_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5036Sink = bytes.LastIndex(ps5036Haystack, ps5036Absent) != -1
	}
}

func BenchmarkPS5036Absent_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5036Sink = bytes.Contains(ps5036Haystack, ps5036Absent)
	}
}
