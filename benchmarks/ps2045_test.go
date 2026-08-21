package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2045 — buf1.String() == buf2.String() vs
// bytes.Equal(buf1.Bytes(), buf2.Bytes()) on two bytes.Buffers. The
// Before side forces Buffer.String() to heap-allocate and byte-copy
// BOTH buffers' entire unread contents before the == even runs; the
// After side reads the two zero-copy Bytes() views and runs one memory
// compare — allocation-free. Both buffers hold the same 4 KiB (the
// shape from the check's MeasuredWin) so the compare runs to the end —
// the worst case for the After side; on a length mismatch it is O(1)
// while the Before side still pays both copies.
var (
	ps2045BufA bytes.Buffer
	ps2045BufB bytes.Buffer
	ps2045Sink bool
)

func init() {
	ps2045BufA.WriteString(strings.Repeat("x", 4096))
	ps2045BufB.WriteString(strings.Repeat("x", 4096))
}

func BenchmarkPS2045_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2045Sink = ps2045BufA.String() == ps2045BufB.String()
	}
}

func BenchmarkPS2045_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2045Sink = bytes.Equal(ps2045BufA.Bytes(), ps2045BufB.Bytes())
	}
}
