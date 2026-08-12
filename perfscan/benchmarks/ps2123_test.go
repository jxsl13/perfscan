package benchmarks

import (
	"fmt"
	"strings"
	"testing"
)

// PS2123 — fmt.Sprint(a, b, c) vs a + b + c. When every operand is a
// plain string Sprint inserts no separators, so the two are
// byte-identical, but Sprint boxes each operand into an interface and
// walks fmt's reflection-based default formatter through a pp buffer
// while + is one direct compiler-emitted concatenation. The operands
// are a realistic host + ":" + port join (24/1/5 bytes).

var (
	ps2123Host = strings.Repeat("h", 15) + ".internal"
	ps2123Sep  = ":"
	ps2123Port = "58080"
)

func BenchmarkPS2123_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = fmt.Sprint(ps2123Host, ps2123Sep, ps2123Port)
	}
}

func BenchmarkPS2123_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = ps2123Host + ps2123Sep + ps2123Port
	}
}
