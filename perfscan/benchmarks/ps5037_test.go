package benchmarks

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// PS5037 — utf8.RuneCountInString/RuneCount compared against 0 for
// emptiness vs the len(...) form the fix emits. The counting functions
// unconditionally scan the ENTIRE input (byte-wise with a
// word-at-a-time ASCII assist, rune-decoding otherwise) to produce an
// exact count that the comparison then collapses to a single bit;
// len(...) reads the string/slice header's length word. The mixed-width
// input exercises the rune-decoding loop, the pure-ASCII input the fast
// path — the win is the whole scan either way and grows with the input.
// The len side never allocates; RuneCount on the mixed bytes also drops
// a 4.8KB/op tail copy (current gc re-materializes string(p[n:]) from
// the first non-ASCII byte — the artifact PS2024 documents).
var (
	ps5037Mixed = strings.Repeat("naïve — 日本語テキスト mixed £ line\n", 96) // ~4.3 KB, mixed widths
	ps5037ASCII = strings.Repeat("service=checkout status=ok\n", 128)  // ~3.5 KB, pure ASCII
	ps5037Bytes = []byte(ps5037Mixed)
)

var ps5037Sink bool

func BenchmarkPS5037String_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5037Sink = utf8.RuneCountInString(ps5037Mixed) == 0
	}
}

func BenchmarkPS5037String_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5037Sink = len(ps5037Mixed) == 0
	}
}

func BenchmarkPS5037ASCII_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5037Sink = utf8.RuneCountInString(ps5037ASCII) == 0
	}
}

func BenchmarkPS5037ASCII_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5037Sink = len(ps5037ASCII) == 0
	}
}

func BenchmarkPS5037Bytes_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5037Sink = utf8.RuneCount(ps5037Bytes) > 0
	}
}

func BenchmarkPS5037Bytes_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5037Sink = len(ps5037Bytes) > 0
	}
}
