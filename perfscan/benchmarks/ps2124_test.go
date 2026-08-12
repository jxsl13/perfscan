package benchmarks

import (
	"strings"
	"testing"
)

// PS2124 — strings.Join([]string{a, b, c}, "/") vs a + "/" + b + "/" + c.
// Join places the separator exactly between consecutive elements, so the
// two are byte-identical, but the Join form builds a throwaway []string
// (stack-allocated here — escape analysis proves Join's parameter does
// not escape — heap-allocated when the literal escapes) and walks it
// twice, while + is one direct compiler-emitted concatenation. The
// operands are a realistic dir + "/" + file + "/" + name path join
// (12/9/17 bytes).

var (
	ps2124Dir  = strings.Repeat("d", 12)
	ps2124File = strings.Repeat("f", 9)
	ps2124Name = strings.Repeat("n", 17)
)

func BenchmarkPS2124_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.Join([]string{ps2124Dir, ps2124File, ps2124Name}, "/")
	}
}

func BenchmarkPS2124_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = ps2124Dir + "/" + ps2124File + "/" + ps2124Name
	}
}
