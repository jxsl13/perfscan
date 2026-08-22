package benchmarks

import (
	"strings"
	"testing"
	"unicode"
)

// PS5034 — strings.FieldsFunc(s, unicode.IsSpace) vs the
// strings.Fields(s) form the fix emits. FieldsFunc always runs the
// general algorithm: it decodes EVERY rune and calls the predicate
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
	ps5034ASCII    = strings.Repeat("level=info msg=quux dur=1.2ms  code=200 \t", 24) // ~1 KB, 96 fields, all ASCII
	ps5034NonASCII = ps5034ASCII + "\u00a0"                                           // one NBSP forces the FieldsFunc delegation
)

var ps5034Sink int

func BenchmarkPS5034ASCII_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = len(strings.FieldsFunc(ps5034ASCII, unicode.IsSpace))
	}
}

func BenchmarkPS5034ASCII_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = len(strings.Fields(ps5034ASCII))
	}
}

func BenchmarkPS5034NonASCII_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = len(strings.FieldsFunc(ps5034NonASCII, unicode.IsSpace))
	}
}

func BenchmarkPS5034NonASCII_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5034Sink = len(strings.Fields(ps5034NonASCII))
	}
}
