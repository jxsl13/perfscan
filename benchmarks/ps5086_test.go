package benchmarks

import (
	"bytes"
	"testing"
)

var (
	ps5086Input = bytes.Repeat([]byte("perfscan-clone-transform-"), 2731)
	ps5086Bytes []byte
)

func BenchmarkPS5086_Before(b *testing.B) {
	for b.Loop() {
		ps5086Bytes = bytes.ToUpper(bytes.Clone(ps5086Input))
	}
}

func BenchmarkPS5086_After(b *testing.B) {
	for b.Loop() {
		ps5086Bytes = bytes.ToUpper(ps5086Input)
	}
}
