package benchmarks

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

// PS2138 — len(bytes.Runes(b)) vs utf8.RuneCount(b). Unlike PS2125's
// len([]rune(s)) CONVERSION arm (which cmd/compile lowers to an
// allocation-free runtime.countrunes call), bytes.Runes is an ordinary
// library call the compiler does NOT special-case: it allocates and fills
// a []rune whose only use here is its len — utf8.RuneCount computes the
// identical integer with zero allocation. The input is a 27-byte
// mixed-width []byte of 17 runes (1-, 2- and 3-byte encodings).

var ps2138Input = []byte("héllo wörld … 日本語")

func BenchmarkPS2138_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = len(bytes.Runes(ps2138Input))
	}
}

func BenchmarkPS2138_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = utf8.RuneCount(ps2138Input)
	}
}
