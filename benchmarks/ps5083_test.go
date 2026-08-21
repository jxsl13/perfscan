package benchmarks

import (
	"bytes"
	"slices"
	"testing"
)

var (
	ps5083Input = bytes.Repeat([]byte("clone-observer-payload-"), 2849)
	ps5083Sink  int
)

func BenchmarkPS5083_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5083Sink = len(bytes.Clone(slices.Clone(bytes.Clone(ps5083Input))))
	}
}

func BenchmarkPS5083_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5083Sink = len(ps5083Input)
	}
}
