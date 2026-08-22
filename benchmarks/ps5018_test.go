package benchmarks

import (
	"strings"
	"testing"
	"unicode"
)

// PS5018 — strings.Map(unicode.ToUpper, s) vs strings.ToUpper(s). The
// Before pays, for every rune, a utf8 decode (the range loop), an
// indirect call through the mapping function pointer into
// unicode.ToUpper, and a re-encode when anything changes. The After
// makes one linear byte scan and, on all-ASCII input needing no
// change, returns s itself with zero per-rune work. Four inputs pin
// the HONEST profile (go1.26): the no-change ASCII path is ~2x faster
// (both spellings return s with 0 allocs, but Map still pays the
// per-rune indirect call to find that out), sparse-change ASCII is
// faster, non-ASCII is exact parity (strings.ToUpper is literally
// defined as this same Map call there) — and change-DENSE ASCII is
// the one class where the After measures ~15-20% SLOWER, because
// unicode.ToUpper's own ASCII fast path keeps Map's indirect call
// cheap while ToUpper's builder segment-copies around every cased
// byte. The check's doc reports that regression rather than hiding
// it; the rewrite stays the canonical stdlib spelling and the win on
// the normalization-dominant no-change path.
var (
	ps5018Sink   string
	ps5018Dense  = "mixed-Case all-ascii payload of Fifty-five Bytes total!"
	ps5018Sparse = "MOSTLY-UPPERCASE ASCII PAYLOAD with A few LOWER bits!!"
	ps5018Upper  = "AN ALREADY-UPPERCASE ASCII LINE THAT NEEDS NO MAPPING!"
	ps5018Greek  = "μη-ascii γραμμή: straße και Ελληνικά — the shared Map path"
)

func BenchmarkPS5018_NoChange_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5018Sink = strings.Map(unicode.ToUpper, ps5018Upper)
	}
}

func BenchmarkPS5018_NoChange_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5018Sink = strings.ToUpper(ps5018Upper)
	}
}

func BenchmarkPS5018_Sparse_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5018Sink = strings.Map(unicode.ToUpper, ps5018Sparse)
	}
}

func BenchmarkPS5018_Sparse_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5018Sink = strings.ToUpper(ps5018Sparse)
	}
}

// The dense-change pair is the honest regression case: the After is
// expected to measure SLOWER here on go1.26 (see the package comment).
func BenchmarkPS5018_Dense_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5018Sink = strings.Map(unicode.ToUpper, ps5018Dense)
	}
}

func BenchmarkPS5018_Dense_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5018Sink = strings.ToUpper(ps5018Dense)
	}
}

func BenchmarkPS5018_NonASCII_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5018Sink = strings.Map(unicode.ToUpper, ps5018Greek)
	}
}

func BenchmarkPS5018_NonASCII_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5018Sink = strings.ToUpper(ps5018Greek)
	}
}
