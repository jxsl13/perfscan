package benchmarks

import (
	"bytes"
	"testing"
	"unicode"
)

// PS3030 — bytes.FieldsFunc(b, unicode.IsSpace) vs the bytes.Fields(b)
// form the fix emits (the bytes twin of PS5034). FieldsFunc always runs
// the general algorithm: it decodes EVERY rune and calls the predicate
// through a function value (an indirect, non-inlinable call) once per
// rune. Fields is defined as that exact call but fronted by an ASCII
// fast path: one pass over the raw bytes through the asciiSpace table —
// no rune decode, no indirect call — that counts and slices the fields
// directly, delegating to FieldsFunc(s, unicode.IsSpace) only when it
// sees a byte >= utf8.RuneSelf. The ASCII pair (a ~1 KB log line with
// 96 fields) shows the win on the dominant input: time AND allocations
// drop, because the fast path skips FieldsFunc's internal span-record
// bookkeeping (whose append growth allocates scratch beyond the result
// slice once the input holds more than 32 fields). The NonASCII pair
// (the same line with one trailing NBSP) bounds the worst case — the
// wasted byte-counting prepass before Fields runs the IDENTICAL
// FieldsFunc call, with identical allocations.
var (
	ps3030ASCII    = bytes.Repeat([]byte("level=info msg=quux dur=1.2ms  code=200 \t"), 24) // ~1 KB, 96 fields, all ASCII
	ps3030NonASCII = append(bytes.Clone(ps3030ASCII), "\u00a0"...)                          // one NBSP forces the FieldsFunc delegation
)

var ps3030Sink int

func BenchmarkPS3030ASCII_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3030Sink = len(bytes.FieldsFunc(ps3030ASCII, unicode.IsSpace))
	}
}

func BenchmarkPS3030ASCII_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3030Sink = len(bytes.Fields(ps3030ASCII))
	}
}

func BenchmarkPS3030NonASCII_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3030Sink = len(bytes.FieldsFunc(ps3030NonASCII, unicode.IsSpace))
	}
}

func BenchmarkPS3030NonASCII_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3030Sink = len(bytes.Fields(ps3030NonASCII))
	}
}
