package benchmarks

import "testing"

// PS2037 — string([]rune{r}) vs string(rune(r)). Both encode exactly one
// rune to UTF-8, but the Before builds a one-element []rune (backing
// array + slice header) and dispatches to the generic
// runtime.slicerunetostring, which makes a counting pass and an encoding
// pass over the slice; the After takes the runtime's direct single-rune
// conversion. The operand cycles ASCII, 3-byte, 4-byte, boundary and
// invalid runes so neither side monomorphizes on one encoding width.

var ps2037Runes = []rune{'A', '日', 0x10FFFF, 0x7F, 0x1F600, -1}

func BenchmarkPS2037_Before(b *testing.B) {
	b.ReportAllocs()
	for i := range b.N {
		sinkS = string([]rune{ps2037Runes[i%len(ps2037Runes)]})
	}
}

func BenchmarkPS2037_After(b *testing.B) {
	b.ReportAllocs()
	for i := range b.N {
		sinkS = string(rune(ps2037Runes[i%len(ps2037Runes)]))
	}
}
