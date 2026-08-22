package benchmarks

import (
	"testing"
	"unicode/utf8"
)

// PS2038 — utf8.DecodeRuneInString(string(b)) vs utf8.DecodeRune(b) (and
// the DecodeLastRune pair), plus the forward mirror
// utf8.DecodeRune([]byte(s)) vs utf8.DecodeRuneInString(s). Each pair
// runs the identical decode core over the same bytes; the reverse Before
// additionally copies the WHOLE slice into a throwaway string to decode
// at most 4 bytes of it. The slice's first byte is toggled between two
// spellings of the same rune-width each iteration so the conversion's
// copy cannot be elided (b is visibly mutated between calls); the
// operand is the 85-byte mixed-width line from the check's MeasuredWin.
// The forward direction demonstrates gc's elision of the non-escaping,
// read-only []byte(s) conversion empirically (near-parity, the honest
// caveat in the check's MeasuredWin).
var (
	ps2038Str   = "wörld … é€\U0001F4A9 service=checkout región=eu-wést-1 status=ok body=héllo, 世界!"
	ps2038Bytes = []byte(ps2038Str)
	ps2038Rune  rune
	ps2038Size  int
)

func BenchmarkPS2038_Before(b *testing.B) {
	b.ReportAllocs()
	for i := range b.N {
		ps2038Bytes[0] = 'w' + byte(i&1)
		ps2038Rune, ps2038Size = utf8.DecodeRuneInString(string(ps2038Bytes))
	}
}

func BenchmarkPS2038_After(b *testing.B) {
	b.ReportAllocs()
	for i := range b.N {
		ps2038Bytes[0] = 'w' + byte(i&1)
		ps2038Rune, ps2038Size = utf8.DecodeRune(ps2038Bytes)
	}
}

func BenchmarkPS2038Last_Before(b *testing.B) {
	b.ReportAllocs()
	for i := range b.N {
		ps2038Bytes[0] = 'w' + byte(i&1)
		ps2038Rune, ps2038Size = utf8.DecodeLastRuneInString(string(ps2038Bytes))
	}
}

func BenchmarkPS2038Last_After(b *testing.B) {
	b.ReportAllocs()
	for i := range b.N {
		ps2038Bytes[0] = 'w' + byte(i&1)
		ps2038Rune, ps2038Size = utf8.DecodeLastRune(ps2038Bytes)
	}
}

func BenchmarkPS2038Forward_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2038Rune, ps2038Size = utf8.DecodeRune([]byte(ps2038Str))
	}
}

func BenchmarkPS2038Forward_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2038Rune, ps2038Size = utf8.DecodeRuneInString(ps2038Str)
	}
}
