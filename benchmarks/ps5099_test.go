package benchmarks

import (
	"io"
	"testing"
)

var (
	ps5099Text = "non-retaining multiwriter consumer"
	ps5099N    int
	ps5099Err  error
)

func BenchmarkPS5099_Before(b *testing.B) {
	for b.Loop() {
		ps5099N, ps5099Err = io.WriteString(
			io.MultiWriter(
				io.MultiWriter(io.Discard, io.Discard),
				io.MultiWriter(io.Discard, io.Discard),
			),
			ps5099Text,
		)
	}
}

func BenchmarkPS5099_After(b *testing.B) {
	for b.Loop() {
		ps5099N, ps5099Err = io.WriteString(
			io.MultiWriter(io.Discard, io.Discard, io.Discard, io.Discard),
			ps5099Text,
		)
	}
}
