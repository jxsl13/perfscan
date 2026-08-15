package benchmarks

import (
	"strings"
	"testing"
)

// PS5030 — strings.IndexAny/ContainsAny with a one-multi-byte-rune
// cutset vs the IndexRune/ContainsRune forms the fix emits. For such a
// cutset (len(chars) >= 2, non-ASCII) IndexAny skips both of its fast
// paths and falls into the general fallback that UTF-8-decodes EVERY
// rune of the haystack and pays a non-inlined IndexRune probe into the
// cutset per rune; IndexRune runs a single substring scan for the rune's
// UTF-8 encoding keyed on its last byte, with no per-rune decode of the
// haystack at all. Zero allocations on every side (the cutset is a
// constant); the win is pure scan cost and grows with the haystack. The
// haystack is a typical log line with the needle rune in the last field,
// the shape from the check's MeasuredWin (and from the PS5022
// benchmarks — the one-ASCII-byte sibling of this rewrite).
var ps5030Haystack = "service=checkout region=eu-west-1 shard=07 cost=42€ final—ok"

var (
	ps5030SinkInt  int
	ps5030SinkBool bool
)

func BenchmarkPS5030Index_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5030SinkInt = strings.IndexAny(ps5030Haystack, "—")
	}
}

func BenchmarkPS5030Index_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5030SinkInt = strings.IndexRune(ps5030Haystack, '—')
	}
}

func BenchmarkPS5030Contains_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5030SinkBool = strings.ContainsAny(ps5030Haystack, "€")
	}
}

func BenchmarkPS5030Contains_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5030SinkBool = strings.ContainsRune(ps5030Haystack, '€')
	}
}
