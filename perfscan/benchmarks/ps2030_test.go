package benchmarks

import (
	"bytes"
	"testing"
)

// PS2030 — len(bytes.Fields(b)) == 0 vs len(bytes.TrimSpace(b)) == 0
// as a blank-slice test (the []byte twin of PS2028). The Before side
// allocates the entire [][]byte of whitespace-delimited fields and
// always scans the whole input just to compare the slice's length and
// throw it away; the After side allocates nothing (TrimSpace returns a
// re-slice) and, for a non-blank input, stops at the first non-space
// byte. Two input shapes: a ~1.2 KB line of 64 space-separated fields
// (the common non-blank case, where TrimSpace short-circuits
// immediately) and an all-whitespace 256-byte input (the blank case,
// where both sides must scan everything and the delta is Fields'
// field-boundary counting work).
var (
	ps2030Line  = bytes.Repeat([]byte("some-field-content "), 64)
	ps2030Blank = bytes.Repeat([]byte(" \t"), 128)
	ps2030Sink  bool
)

func BenchmarkPS2030_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2030Sink = len(bytes.Fields(ps2030Line)) == 0
	}
}

func BenchmarkPS2030_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2030Sink = len(bytes.TrimSpace(ps2030Line)) == 0
	}
}

func BenchmarkPS2030Blank_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2030Sink = len(bytes.Fields(ps2030Blank)) == 0
	}
}

func BenchmarkPS2030Blank_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2030Sink = len(bytes.TrimSpace(ps2030Blank)) == 0
	}
}
