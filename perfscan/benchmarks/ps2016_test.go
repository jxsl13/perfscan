package benchmarks

import (
	"bytes"
	"strings"
	"testing"
	"unicode"
)

// PS2016 — string(bytes.TrimFunc([]byte(s), f)) vs
// strings.TrimFunc(s, f). The Before round-trips s through []byte: the
// []byte(s) conversion copies s (heap or stack, depending on what
// escape analysis can prove for the shape — here the predicate is an
// opaque func value, so the conversion typically stays a real copy),
// and the string(...) of the trimmed subslice is always a fresh heap
// copy. The After returns a substring of s: the identical rune-by-rune
// trim calling the SAME predicate on the SAME runes, zero allocations,
// zero copies. The input is a typical config/log line with whitespace
// on both ends so the trim does real work at both edges; the predicate
// is unicode.IsSpace, the common production case.
var ps2016Line = "  \t--key = some configuration value with padding--\t\r\n"

func BenchmarkPS2016_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.TrimFunc([]byte(ps2016Line), unicode.IsSpace))
	}
}

func BenchmarkPS2016_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.TrimFunc(ps2016Line, unicode.IsSpace)
	}
}
