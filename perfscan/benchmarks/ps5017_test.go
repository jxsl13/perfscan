package benchmarks

import (
	"bytes"
	"testing"
	"unicode"
)

// PS5017 — bytes.Map(unicode.ToUpper, b) vs bytes.ToUpper(b). The
// Before pays, for every rune, a utf8.DecodeRune, an indirect call
// through the mapping function pointer into unicode.ToUpper's caseRange
// table walk, and a utf8.AppendRune. The After makes one linear scan
// and, on all-ASCII input, either memmove-copies b unchanged or runs a
// tight 'a'<=c<='z' byte loop — no decode, no indirect call, no table
// walk. Three inputs bracket the win: an all-ASCII mixed-case line (the
// fast-path byte loop), an already-uppercase ASCII line (the plain-copy
// path), and a non-ASCII line, where bytes.ToUpper is literally defined
// as this same Map call and the two spellings measure at parity.
var (
	ps5017Sink  []byte
	ps5017Mixed = []byte("mixed-Case all-ascii payload of Fifty-five Bytes total!")
	ps5017Upper = []byte("AN ALREADY-UPPERCASE ASCII LINE THAT NEEDS NO MAPPING!")
	ps5017Greek = []byte("μη-ascii γραμμή: straße και Ελληνικά — the shared Map path")
)

func BenchmarkPS5017_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5017Sink = bytes.Map(unicode.ToUpper, ps5017Mixed)
	}
}

func BenchmarkPS5017_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5017Sink = bytes.ToUpper(ps5017Mixed)
	}
}

func BenchmarkPS5017_NoChange_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5017Sink = bytes.Map(unicode.ToUpper, ps5017Upper)
	}
}

func BenchmarkPS5017_NoChange_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5017Sink = bytes.ToUpper(ps5017Upper)
	}
}

func BenchmarkPS5017_NonASCII_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5017Sink = bytes.Map(unicode.ToUpper, ps5017Greek)
	}
}

func BenchmarkPS5017_NonASCII_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5017Sink = bytes.ToUpper(ps5017Greek)
	}
}
