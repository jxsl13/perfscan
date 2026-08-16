package benchmarks

import (
	"encoding/hex"
	"fmt"
	"testing"
)

// PS2046 — fmt.Appendf(buf, "%x", bs) vs hex.AppendEncode(buf, bs). For a
// lone lowercase %x over a []byte the two append the identical lowercase
// hex digits, but Appendf parses the format, BOXES bs into an interface (a
// heap allocation) and walks fmt's pp buffer, while hex.AppendEncode runs a
// tight encode loop straight into buf's existing capacity. Both reuse a
// preallocated destination, so the only allocation is Appendf's interface
// box, which hex.AppendEncode removes entirely. (The check is advisory —
// the rewrite diverges when bs overlaps buf's spare capacity — but the win
// it advises is real, measured here.)
var (
	ps2046Buf  = make([]byte, 0, 256)
	ps2046Bs   = []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	ps2046Sink []byte
)

func BenchmarkPS2046_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2046Sink = fmt.Appendf(ps2046Buf[:0], "%x", ps2046Bs)
	}
}

func BenchmarkPS2046_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2046Sink = hex.AppendEncode(ps2046Buf[:0], ps2046Bs)
	}
}
