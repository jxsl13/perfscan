package benchmarks

import (
	"fmt"
	"testing"
)

// PS2033 — fmt.Appendf(buf, "%s%s", a, b) vs append(append(buf, a...), b...).
// For a format that is nothing but %s verbs over plain strings the two append
// the identical bytes, but Appendf BOXES EACH operand into an interface (one
// heap allocation per operand), parses the format and walks fmt's pp buffer,
// while the chain is two builtin string->[]byte copies into buf's existing
// capacity. Both reuse a preallocated destination, so the only allocations are
// Appendf's interface boxes, which the chain removes entirely.
var (
	ps2033Buf  = make([]byte, 0, 256)
	ps2033A    = "the quick br"
	ps2033B    = "own fox jumps"
	ps2033Sink []byte
)

func BenchmarkPS2033_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2033Sink = fmt.Appendf(ps2033Buf[:0], "%s%s", ps2033A, ps2033B)
	}
}

func BenchmarkPS2033_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2033Sink = append(append(ps2033Buf[:0], ps2033A...), ps2033B...)
	}
}
