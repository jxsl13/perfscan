package benchmarks

import (
	"bytes"
	"testing"
)

var (
	ps5094Input = bytes.Repeat([]byte("ephemeral-buffer-string-"), 2731)
	ps5094Sink  string
)

func BenchmarkPS5094_Before(b *testing.B) {
	for b.Loop() {
		ps5094Sink = bytes.NewBuffer(bytes.Clone(ps5094Input)).String()
	}
}

func BenchmarkPS5094_After(b *testing.B) {
	for b.Loop() {
		ps5094Sink = string(ps5094Input)
	}
}
