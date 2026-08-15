package benchmarks

import (
	"strings"
	"testing"
)

// PS2028 — len(strings.Fields(s)) == 0 vs strings.TrimSpace(s) == ""
// as a blank-string test. The Before side allocates the entire
// []string of whitespace-delimited fields and always scans the whole
// input just to compare the slice's length and throw it away; the
// After side allocates nothing (TrimSpace returns a re-slice) and, for
// a non-blank input, stops at the first non-space byte. Two input
// shapes: a ~1.2 KB line of 64 space-separated fields (the common
// non-blank case, where TrimSpace short-circuits immediately) and an
// all-whitespace 256-byte input (the blank case, where both sides must
// scan everything and the delta is Fields' counting work).
var (
	ps2028Line  = strings.Repeat("some-field-content ", 64)
	ps2028Blank = strings.Repeat(" \t", 128)
	ps2028Sink  bool
)

func BenchmarkPS2028_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2028Sink = len(strings.Fields(ps2028Line)) == 0
	}
}

func BenchmarkPS2028_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2028Sink = strings.TrimSpace(ps2028Line) == ""
	}
}

func BenchmarkPS2028Blank_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2028Sink = len(strings.Fields(ps2028Blank)) == 0
	}
}

func BenchmarkPS2028Blank_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2028Sink = strings.TrimSpace(ps2028Blank) == ""
	}
}
