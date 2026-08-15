package benchmarks

import (
	"fmt"
	"testing"
)

// PS3025 — fmt.Appendf(buf, "constant") vs append(buf, "constant"...). With no
// verbs and no operands fmt.Appendf's whole pipeline (pool round-trip,
// byte-by-byte format scan, pp-buffer copy, final append) exists only to append
// the literal's own bytes; the builtin append is a single memmove into buf's
// existing capacity. Both reuse a preallocated destination, so neither
// allocates — the win is the removed fmt machinery.
var (
	ps3025Buf  = make([]byte, 0, 256)
	ps3025Sink []byte
)

func BenchmarkPS3025_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3025Sink = fmt.Appendf(ps3025Buf[:0], "HTTP/1.1 200 OK\r\n")
	}
}

func BenchmarkPS3025_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps3025Sink = append(ps3025Buf[:0], "HTTP/1.1 200 OK\r\n"...)
	}
}
