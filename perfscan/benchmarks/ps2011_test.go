package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

// PS2011 (advisory) — []byte(strings.Repeat(s, n)) vs
// bytes.Repeat([]byte(s), n). Before allocates and fills the repeated
// STRING, then the []byte(...) conversion allocates a second buffer and
// copies every byte into it: two allocations, twice the bytes touched.
// After converts only the short seed and fills a single []byte buffer.
// The saving scales with the output length (here 128 bytes). The check is
// advisory — capacity and panic-message divergences block an auto-fix
// (see checks/equiv_PS2011_test.go) — but the win the advisory promises
// is measured here.

var ps2011Sink []byte

func BenchmarkPS2011_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2011Sink = []byte(strings.Repeat("ab", 64))
	}
}

func BenchmarkPS2011_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2011Sink = bytes.Repeat([]byte("ab"), 64)
	}
}
