package benchmarks

import (
	"fmt"
	"testing"
)

// PS2044 — fmt.Appendf(buf, "%s=%s;", k, v) vs
// append(append(append(append(buf, k...), "="...), v...), ";"...).
// For a format that is literal text spliced with bare %s verbs over plain
// strings the two append the identical bytes, but Appendf BOXES EACH operand
// into an interface (one heap allocation per operand), parses the format and
// walks fmt's pp buffer, while the chain is four builtin string->[]byte
// copies into buf's existing capacity (two of them constant literals). Both
// reuse a preallocated destination, so the only allocations are Appendf's
// interface boxes, which the chain removes entirely.
var (
	ps2044Buf  = make([]byte, 0, 256)
	ps2044K    = "content-type"
	ps2044V    = "text/plain;utf8"
	ps2044Sink []byte
)

func BenchmarkPS2044_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2044Sink = fmt.Appendf(ps2044Buf[:0], "%s=%s;", ps2044K, ps2044V)
	}
}

func BenchmarkPS2044_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2044Sink = append(append(append(append(ps2044Buf[:0], ps2044K...), "="...), ps2044V...), ";"...)
	}
}
