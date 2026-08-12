package benchmarks

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// PS2125 — len([]rune(s)) vs utf8.RuneCountInString(s), and
// len([]byte(s)) vs len(s). Both pairs measure IDENTICAL on current gc:
// cmd/compile special-cases exactly these compositions, lowering
// len([]rune(s)) to an allocation-free runtime.countrunes call and
// len([]byte(s)) to len(s). The benchmarks pin that equivalence (the
// check's win on gc is source-level robustness — a conversion that
// drifts out of the peephole's exact shape allocates for real — and a
// genuine allocation removal on toolchains without the peephole). The
// line is a realistic ~1.1KB mixed-width string (896 runes, 1152 bytes).

var ps2125Line = strings.Repeat("héllo wörld … ", 64)

func BenchmarkPS2125Rune_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = len([]rune(ps2125Line))
	}
}

func BenchmarkPS2125Rune_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = utf8.RuneCountInString(ps2125Line)
	}
}

func BenchmarkPS2125Byte_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = len([]byte(ps2125Line))
	}
}

func BenchmarkPS2125Byte_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkI = len(ps2125Line)
	}
}
