package benchmarks

import (
	"bytes"
	"encoding/base64"
	"testing"
)

var (
	ps5058A = bytes.Repeat([]byte("perfscan"), 4) // 32 bytes
	ps5058B = append(bytes.Repeat([]byte("perfscan"), 4), 'x')
	ps5058R bool
)

// BenchmarkPS5058Before is enc.EncodeToString(a) == enc.EncodeToString(b): two
// encoding allocations and two passes.
func BenchmarkPS5058Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5058R = base64.StdEncoding.EncodeToString(ps5058A) == base64.StdEncoding.EncodeToString(ps5058B)
	}
}

// BenchmarkPS5058After is bytes.Equal(a, b): a direct byte scan, no allocation.
func BenchmarkPS5058After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5058R = bytes.Equal(ps5058A, ps5058B)
	}
}
