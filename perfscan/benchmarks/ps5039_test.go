package benchmarks

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// PS5039 — append(dst, string(r)...) vs utf8.AppendRune(dst, r) for a
// non-constant rune. The Before spelling UTF-8-encodes r into a
// temporary string (runtime.intstring: encoder run + string header) and
// append then copies those bytes a second time into dst; AppendRune
// runs the same encoder once, straight into dst's backing array. With
// dst preallocated both sides are allocation-free here (the temporary
// string stack-spills when it does not escape) — the win is the dropped
// encode-into-string + copy round trip, and it widens further when the
// temporary escapes and heap-allocates per multi-byte rune.
var (
	ps5039Runes = func() []rune {
		var out []rune
		for len(out) < 1024 {
			out = append(out, []rune("aZ0~ éß£¿ … 日本語テキスト 𝄞😀🚀 �")...)
		}
		return out[:1024]
	}()
	ps5039ASCII = []rune(strings.Repeat("service=checkout status=ok\n", 40))[:1024]
	ps5039Dst   = make([]byte, 0, 4*1024)
)

var ps5039Sink []byte

func BenchmarkPS5039Mixed_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		dst := ps5039Dst[:0]
		for _, r := range ps5039Runes {
			dst = append(dst, string(r)...) //perfscan:ignore PS5039 the Before shape this benchmark exists to measure
		}
		ps5039Sink = dst
	}
}

func BenchmarkPS5039Mixed_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		dst := ps5039Dst[:0]
		for _, r := range ps5039Runes {
			dst = utf8.AppendRune(dst, r)
		}
		ps5039Sink = dst
	}
}

func BenchmarkPS5039ASCII_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		dst := ps5039Dst[:0]
		for _, r := range ps5039ASCII {
			dst = append(dst, string(r)...) //perfscan:ignore PS5039 the Before shape this benchmark exists to measure
		}
		ps5039Sink = dst
	}
}

func BenchmarkPS5039ASCII_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		dst := ps5039Dst[:0]
		for _, r := range ps5039ASCII {
			dst = utf8.AppendRune(dst, r)
		}
		ps5039Sink = dst
	}
}
