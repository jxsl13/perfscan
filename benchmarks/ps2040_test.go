package benchmarks

import (
	"fmt"
	"testing"
)

// PS2040 — fmt.Append(buf, host, ":", port) vs the nested chain
// append(append(append(buf, host...), ":"...), port...). With every operand a
// string, Sprint's between-operand spacing rule never applies and the two
// append the identical bytes, but fmt.Append BOXES EACH operand into an
// interface (one heap allocation per VARIABLE operand; the constant ":" boxes
// into a static interface value at compile time), walks fmt's printer through
// a pooled pp buffer and copies that buffer onto buf, while the chain is three
// builtin string->[]byte copies into buf's existing capacity. Both reuse a
// preallocated destination, so the only allocations are Append's interface
// boxes, which the chain removes entirely.
var (
	ps2040Buf  = make([]byte, 0, 256)
	ps2040Host = "example.co"
	ps2040Port = "8080"
	ps2040Sink []byte
)

func BenchmarkPS2040_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2040Sink = fmt.Append(ps2040Buf[:0], ps2040Host, ":", ps2040Port)
	}
}

func BenchmarkPS2040_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2040Sink = append(append(append(ps2040Buf[:0], ps2040Host...), ":"...), ps2040Port...)
	}
}
