package benchmarks

import (
	"fmt"
	"testing"
)

// PS2041 — fmt.Appendln(buf, a, b) vs the nested chain
// append(append(append(append(buf, a...), ' '), b...), '\n'). For plain-string
// operands the two append the identical bytes (doPrintln's separator rule is
// unconditional: one ' ' between operands, one trailing '\n'), but Appendln
// BOXES EACH operand into an interface (one heap allocation per operand),
// takes a pp printer from fmt's sync.Pool and formats through its pooled
// buffer, while the chain is builtin string->[]byte copies plus two
// single-byte appends into buf's existing capacity. Both reuse a preallocated
// destination, so the only allocations are Appendln's interface boxes, which
// the chain removes entirely.
var (
	ps2041Buf  = make([]byte, 0, 256)
	ps2041A    = "the quick br"
	ps2041B    = "own fox jumps"
	ps2041Sink []byte
)

func BenchmarkPS2041_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2041Sink = fmt.Appendln(ps2041Buf[:0], ps2041A, ps2041B)
	}
}

func BenchmarkPS2041_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2041Sink = append(append(append(append(ps2041Buf[:0], ps2041A...), ' '), ps2041B...), '\n')
	}
}
