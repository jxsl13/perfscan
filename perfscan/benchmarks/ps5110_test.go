package benchmarks

import (
	"bytes"
	"slices"
	"testing"
)

var (
	ps5110A    = bytes.Repeat([]byte{0x11}, 16<<10)
	ps5110B    = bytes.Repeat([]byte{0x22}, 16<<10)
	ps5110C    = bytes.Repeat([]byte{0x33}, 16<<10)
	ps5110D    = bytes.Repeat([]byte{0x44}, 16<<10)
	ps5110Sink []byte
)

func BenchmarkPS5110_Before(b *testing.B) {
	for b.Loop() {
		ps5110Sink = slices.Concat(slices.Concat(ps5110A, ps5110B), slices.Concat(ps5110C, ps5110D))
	}
}

func BenchmarkPS5110_After(b *testing.B) {
	for b.Loop() {
		ps5110Sink = slices.Concat(ps5110A, ps5110B, ps5110C, ps5110D)
	}
}
