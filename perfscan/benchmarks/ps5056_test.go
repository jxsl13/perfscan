package benchmarks

import (
	"encoding/hex"
	"testing"
)

var (
	ps5056In   = make([]byte, 64)
	ps5056Sink []byte
)

// BenchmarkPS5056Before is []byte(hex.EncodeToString(b)): a hex string plus a
// copy into a fresh []byte — two allocations.
func BenchmarkPS5056Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5056Sink = []byte(hex.EncodeToString(ps5056In))
	}
}

// BenchmarkPS5056After is hex.AppendEncode([]byte{}, b): encoded straight into a
// []byte — one allocation, no intermediate string.
func BenchmarkPS5056After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5056Sink = hex.AppendEncode([]byte{}, ps5056In)
	}
}
