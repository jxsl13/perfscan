package benchmarks

import (
	"bytes"
	"testing"
)

var (
	ps5108Seed = bytes.Repeat([]byte("0123456789abcdef"), 256)
	ps5108Sink []byte
)

func BenchmarkPS5108_Before(b *testing.B) {
	for b.Loop() {
		ps5108Sink = bytes.Repeat(bytes.Repeat(ps5108Seed, 4), 4)
	}
}

func BenchmarkPS5108_After(b *testing.B) {
	for b.Loop() {
		ps5108Sink = bytes.Repeat(ps5108Seed, 16)
	}
}
