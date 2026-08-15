package benchmarks

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// PS2024 — utf8.RuneCount([]byte(s)) vs utf8.RuneCountInString(s), and
// the mirror utf8.RuneCountInString(string(b)) vs utf8.RuneCount(b).
// The count is the same loop in both spellings; the Before pays a full
// heap copy of the operand per call (the operand is well past any stack
// temporary), the After at most RuneCount's internal tail copy from the
// first non-ASCII byte (see the check's Doc). Three pairs pin the three
// regimes: the string direction on a realistic ~1.1KB mixed-width line
// (896 runes, 1152 bytes; full alloc -> zero), the mirror on
// ASCII-dominated bytes (full alloc -> zero, plus RuneCount's ~2x
// byte-wise ASCII fast path), and the mirror on the same mixed line
// whose second byte is already non-ASCII (alloc parity — the honest
// worst case, where the tail is nearly the whole input).

var (
	ps2024Line       = strings.Repeat("héllo wörld … ", 64)
	ps2024MixedBytes = []byte(ps2024Line)
	ps2024AsciiBytes = []byte(strings.Repeat("abcdefgh", 143) + "é")
)

func BenchmarkPS2024String_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = utf8.RuneCount([]byte(ps2024Line))
	}
}

func BenchmarkPS2024String_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = utf8.RuneCountInString(ps2024Line)
	}
}

func BenchmarkPS2024BytesAscii_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = utf8.RuneCountInString(string(ps2024AsciiBytes))
	}
}

func BenchmarkPS2024BytesAscii_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = utf8.RuneCount(ps2024AsciiBytes)
	}
}

func BenchmarkPS2024BytesMixed_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = utf8.RuneCountInString(string(ps2024MixedBytes))
	}
}

func BenchmarkPS2024BytesMixed_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = utf8.RuneCount(ps2024MixedBytes)
	}
}
