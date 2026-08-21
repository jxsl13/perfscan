package benchmarks

import (
	"bytes"
	"testing"
)

var (
	ps5074Input = bytes.Repeat([]byte("perfscan"), 8*1024) // 64 KiB
	ps5074Sink  []byte
)

func BenchmarkPS5074_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5074Sink = bytes.Clone(bytes.Clone(ps5074Input))
	}
}

func BenchmarkPS5074_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5074Sink = bytes.Clone(ps5074Input)
	}
}
