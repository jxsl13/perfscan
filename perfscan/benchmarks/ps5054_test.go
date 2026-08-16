package benchmarks

import (
	"bytes"
	"encoding/hex"
	"testing"
)

var (
	ps5054A = bytes.Repeat([]byte("perfscan"), 4) // 32 bytes
	ps5054B = append(bytes.Repeat([]byte("perfscan"), 4), 'x')
	ps5054R bool
)

// BenchmarkPS5054Before is the hex.EncodeToString(a) == hex.EncodeToString(b)
// form the check flags: two 2*len-byte encoding allocations and two passes.
func BenchmarkPS5054Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5054R = hex.EncodeToString(ps5054A) == hex.EncodeToString(ps5054B)
	}
}

// BenchmarkPS5054After is the bytes.Equal(a, b) rewrite: a direct byte scan
// with an early length check, no allocation.
func BenchmarkPS5054After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5054R = bytes.Equal(ps5054A, ps5054B)
	}
}
