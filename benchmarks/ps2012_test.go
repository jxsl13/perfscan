package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2012 — string(bytes.TrimSpace([]byte(s))) vs strings.TrimSpace(s).
// The Before round-trips s through []byte: on current gc, escape
// analysis stack-allocates the []byte(s) conversion in this exact shape
// (bytes.TrimSpace only returns a subslice, which the string conversion
// merely reads), so the measured cost is ONE heap copy — the string(...)
// of the trimmed subslice — plus the full-length byte copy into the
// stack buffer. In shapes escape analysis cannot prove, and on
// toolchains without the optimization, both copies hit the heap. The
// After returns a substring of s: the identical trim scan, zero
// allocations, zero copies. The input is a typical config/log line with
// whitespace on both ends so the trim does real work at both edges.
var ps2012Line = "  \t  key = some configuration value with padding  \r\n  "

func BenchmarkPS2012_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.TrimSpace([]byte(ps2012Line)))
	}
}

func BenchmarkPS2012_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.TrimSpace(ps2012Line)
	}
}
