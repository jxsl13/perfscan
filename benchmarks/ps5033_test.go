package benchmarks

import (
	"fmt"
	"testing"
)

// PS5033 — fmt.Append(buf, s) vs append(buf, s...). For a single plain-string
// operand the two append the identical bytes, but fmt.Append BOXES s into an
// interface (a heap allocation), acquires a pp printer from fmt's sync.Pool,
// formats through the pooled buffer and copies it out, while append is a
// single builtin copy into buf's existing capacity. Both reuse a preallocated
// destination, so the only allocation is Append's interface box, which append
// removes entirely. The plain-Append sibling of PS2141's Appendf("%s") pair.
var (
	ps5033Buf  = make([]byte, 0, 256)
	ps5033S    = "the quick brown fox jumps"
	ps5033Sink []byte
)

func BenchmarkPS5033_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5033Sink = fmt.Append(ps5033Buf[:0], ps5033S)
	}
}

func BenchmarkPS5033_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5033Sink = append(ps5033Buf[:0], ps5033S...)
	}
}
