package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS5005 — string(bytes.Trim([]byte(s), cutset)) vs
// strings.Trim(s, cutset). The Before round-trips s through []byte: on
// current gc, escape analysis stack-allocates the []byte(s) conversion
// in this exact shape (bytes.Trim only returns a subslice, which the
// string conversion merely reads), so the measured cost is ONE heap
// copy — the string(...) of the trimmed subslice — plus the full-length
// byte copy into the stack buffer. In shapes escape analysis cannot
// prove, and on toolchains without the optimization, both copies hit
// the heap. The After returns a substring of s: the identical trim
// scan, zero allocations, zero copies. The input is a typical
// config/log line with cutset members on both ends so the trim does
// real work at both edges; the cutset takes the ASCII-set membership
// path, the common production case.
var (
	ps5005Line   = "  \t--key = some configuration value with padding--\t\r\n"
	ps5005Cutset = " \t\r\n-"
)

func BenchmarkPS5005_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = string(bytes.Trim([]byte(ps5005Line), ps5005Cutset))
	}
}

func BenchmarkPS5005_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strings.Trim(ps5005Line, ps5005Cutset)
	}
}
