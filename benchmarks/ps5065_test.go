package benchmarks

import (
	"bytes"
	"encoding/hex"
	"testing"
)

var (
	ps5065A = bytes.Repeat([]byte("perfscan"), 4) // 32 bytes
	ps5065B = append(bytes.Repeat([]byte("perfscan"), 4), 'x')
	ps5065R bool
)

// BenchmarkPS5065Before is hex.EncodeToString(a) < hex.EncodeToString(b): two
// hex-string allocations and two encoding passes just to order the slices.
func BenchmarkPS5065Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5065R = hex.EncodeToString(ps5065A) < hex.EncodeToString(ps5065B)
	}
}

// BenchmarkPS5065After is bytes.Compare(a, b) < 0: a direct byte comparison, no
// allocation.
func BenchmarkPS5065After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5065R = bytes.Compare(ps5065A, ps5065B) < 0
	}
}
