package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS5006 — string(bytes.TrimPrefix([]byte(s), []byte(p))) vs
// strings.TrimPrefix(s, p). The Before round-trips s and p through
// []byte: on current gc, escape analysis stack-allocates both
// conversions (bytes.TrimPrefix only reslices its inputs), so the
// measured cost is ONE heap copy — the mandatory string(...) of the
// trimmed subslice ('string(~r0) escapes to heap') — plus the
// full-length byte copies into the stack buffers. The After returns a
// substring of s: the identical byte-wise comparison, zero allocations,
// zero copies. The input is a typical 53-byte header line with a
// 16-byte prefix, the shape from the check's MeasuredWin.
var (
	ps5006Line   = "request-header: value payload for the downstream call"
	ps5006Prefix = "request-header: "
)

func BenchmarkPS5006_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.TrimPrefix([]byte(ps5006Line), []byte(ps5006Prefix)))
	}
}

func BenchmarkPS5006_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.TrimPrefix(ps5006Line, ps5006Prefix)
	}
}
