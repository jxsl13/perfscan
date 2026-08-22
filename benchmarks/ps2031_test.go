package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2031 — buf.String() == "OK" vs bytes.Equal(buf.Bytes(),
// []byte("OK")) on a bytes.Buffer. The Before side forces
// Buffer.String() to heap-allocate and byte-copy the entire unread
// contents, only for the comparison to short-circuit on the length
// mismatch against the two-byte constant; the After side reads the
// zero-copy Bytes() view, converts the constant on the stack (it does
// not escape into Equal), and bails out on the length test — O(1) and
// allocation-free. The buffer holds 4 KiB (the shape from the check's
// MeasuredWin); the Before cost grows linearly with the buffered data.
var (
	ps2031Buf  bytes.Buffer
	ps2031Sink bool
)

func init() { ps2031Buf.WriteString(strings.Repeat("x", 4096)) }

func BenchmarkPS2031_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2031Sink = ps2031Buf.String() == "OK"
	}
}

func BenchmarkPS2031_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2031Sink = bytes.Equal(ps2031Buf.Bytes(), []byte("OK"))
	}
}
