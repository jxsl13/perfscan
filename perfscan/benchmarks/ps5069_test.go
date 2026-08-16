package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS5069 — strings.HasPrefix(buf.String(), "xy") vs
// bytes.HasPrefix(buf.Bytes(), []byte("xy")) on a bytes.Buffer. The
// Before side forces Buffer.String() to heap-allocate and byte-copy the
// entire unread contents before the predicate runs; the After side reads
// the zero-copy Bytes() view and converts the tiny constant on the stack
// (it does not escape into the read-only predicate) — allocation-free.
// The buffer holds 4 KiB (the shape from the check's MeasuredWin); the
// Before cost grows linearly with the buffered data. strings.Contains/
// Index/Count show the same B/op collapse.
var (
	ps5069Buf  bytes.Buffer
	ps5069Sink bool
)

func init() { ps5069Buf.WriteString(strings.Repeat("x", 4096)) }

func BenchmarkPS5069_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5069Sink = strings.HasPrefix(ps5069Buf.String(), "xy")
	}
}

func BenchmarkPS5069_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5069Sink = bytes.HasPrefix(ps5069Buf.Bytes(), []byte("xy"))
	}
}
