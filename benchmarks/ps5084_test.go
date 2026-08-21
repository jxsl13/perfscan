package benchmarks

import (
	"bytes"
	"testing"
)

var (
	ps5084Source = bytes.Repeat([]byte("clone-before-copy-payload-"), 2521)
	ps5084Dest   = make([]byte, len(ps5084Source))
	ps5084N      int
)

func BenchmarkPS5084_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5084N = copy(ps5084Dest, bytes.Clone(ps5084Source))
	}
}

func BenchmarkPS5084_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5084N = copy(ps5084Dest, ps5084Source)
	}
}
