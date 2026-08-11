package benchmarks

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// PS2115 — []rune(s)[0] decodes and allocates the whole rune slice to
// read one rune vs utf8.DecodeRuneInString decoding just the first.
//
// ~79 runes of mixed-width text: longer than the compiler's 32-rune
// stack buffer, so the Before conversion heap-allocates on every read.
var ps2115Line = "état: größe µ-bench — " + strings.Repeat("déjà-vu ", 7)

func BenchmarkPS2115_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = int([]rune(ps2115Line)[0])
	}
}

func BenchmarkPS2115_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		r, _ := utf8.DecodeRuneInString(ps2115Line)
		sinkI = int(r)
	}
}
