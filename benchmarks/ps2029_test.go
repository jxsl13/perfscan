package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2029 — len(strings.SplitN(s, sep, 2)) == 2 vs
// strings.Contains(s, sep) as a membership test. Both sides scan to
// the first separator occurrence with the same Index machinery; the
// Before side additionally allocates the two-slot piece slice (a
// []string with two string headers — for the bytes twin a [][]byte
// with two slice headers) just to have its length compared and the
// slice thrown away. The input is a ~1.6 KB line with the two-byte
// separator in the middle, so the scan cost is identical on both
// sides and the measured delta is exactly the allocation the rewrite
// removes.
var (
	ps2029Line  = strings.Repeat("key-", 100) + "=>" + strings.Repeat("-value", 100)
	ps2029BLine = []byte(ps2029Line)
	ps2029BSep  = []byte("=>")
	ps2029Sink  bool
)

func BenchmarkPS2029_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2029Sink = len(strings.SplitN(ps2029Line, "=>", 2)) == 2
	}
}

func BenchmarkPS2029_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2029Sink = strings.Contains(ps2029Line, "=>")
	}
}

func BenchmarkPS2029Bytes_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2029Sink = len(bytes.SplitN(ps2029BLine, ps2029BSep, 2)) == 2
	}
}

func BenchmarkPS2029Bytes_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2029Sink = bytes.Contains(ps2029BLine, ps2029BSep)
	}
}
