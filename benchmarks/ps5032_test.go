package benchmarks

import (
	"bytes"
	"testing"
)

// PS5032 — bytes.IndexAny/ContainsAny with a one-multi-byte-rune
// cutset vs the IndexRune/ContainsRune forms the fix emits. For such a
// cutset (len(chars) >= 2, non-ASCII) IndexAny skips both of its fast
// paths and falls into the general loop that UTF-8-decodes EVERY
// non-ASCII position of the haystack (and pays a per-byte cutset probe
// for every ASCII byte); IndexRune runs a single substring scan for the
// rune's UTF-8 encoding keyed on its last byte, with no per-rune decode
// of the haystack at all. Zero allocations on every side (the cutset is
// a constant); the win is pure scan cost and grows with the haystack.
// The haystack is a typical log line with the needle rune in the last
// field, the shape from the check's MeasuredWin (and from the PS5030
// benchmarks — the strings twin of this rewrite).
var ps5032Haystack = []byte("service=checkout region=eu-west-1 shard=07 cost=42€ final—ok")

var (
	ps5032SinkInt  int
	ps5032SinkBool bool
)

func BenchmarkPS5032Index_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5032SinkInt = bytes.IndexAny(ps5032Haystack, "—")
	}
}

func BenchmarkPS5032Index_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5032SinkInt = bytes.IndexRune(ps5032Haystack, '—')
	}
}

func BenchmarkPS5032Contains_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5032SinkBool = bytes.ContainsAny(ps5032Haystack, "€")
	}
}

func BenchmarkPS5032Contains_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5032SinkBool = bytes.ContainsRune(ps5032Haystack, '€')
	}
}
