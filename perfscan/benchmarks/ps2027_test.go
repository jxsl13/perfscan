package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2027 — buf.String() == "" (and len(buf.String()) > 0) vs
// buf.Len() == 0 (resp. buf.Len() > 0) on a bytes.Buffer. The Before
// sides force Buffer.String() to heap-allocate and byte-copy the entire
// unread contents just to answer a one-word emptiness question; the
// After sides read len(b.buf)-b.off, a single O(1) integer test with no
// allocation. The buffer holds 4 KiB (the shape from the check's
// MeasuredWin); the Before cost grows linearly with the buffered data.
var (
	ps2027Buf  bytes.Buffer
	ps2027Sink bool
)

func init() { ps2027Buf.WriteString(strings.Repeat("x", 4096)) }

func BenchmarkPS2027_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2027Sink = ps2027Buf.String() == ""
	}
}

func BenchmarkPS2027_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2027Sink = ps2027Buf.Len() == 0
	}
}

func BenchmarkPS2027Len_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2027Sink = len(ps2027Buf.String()) > 0
	}
}

func BenchmarkPS2027Len_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2027Sink = ps2027Buf.Len() > 0
	}
}
