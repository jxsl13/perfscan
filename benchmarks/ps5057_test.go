package benchmarks

import (
	"encoding/base64"
	"testing"
)

var (
	ps5057In   = make([]byte, 48)
	ps5057Sink []byte
)

// BenchmarkPS5057Before is []byte(base64.StdEncoding.EncodeToString(b)): an
// encoded string plus a copy into a fresh []byte — two allocations.
func BenchmarkPS5057Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5057Sink = []byte(base64.StdEncoding.EncodeToString(ps5057In))
	}
}

// BenchmarkPS5057After is base64.StdEncoding.AppendEncode([]byte{}, b): encoded
// straight into a []byte — one allocation, no intermediate string.
func BenchmarkPS5057After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5057Sink = base64.StdEncoding.AppendEncode([]byte{}, ps5057In)
	}
}
