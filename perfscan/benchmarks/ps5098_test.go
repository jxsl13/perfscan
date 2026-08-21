package benchmarks

import (
	"io"
	"testing"
)

var (
	ps5098Payload = []byte("terminal multiwriter payload")
	ps5098N       int
	ps5098Err     error
)

func BenchmarkPS5098_Before(b *testing.B) {
	for b.Loop() {
		ps5098N, ps5098Err = io.MultiWriter(
			io.MultiWriter(io.Discard, io.Discard),
			io.MultiWriter(io.Discard, io.Discard),
		).Write(ps5098Payload)
	}
}

func BenchmarkPS5098_After(b *testing.B) {
	for b.Loop() {
		ps5098N, ps5098Err = io.MultiWriter(
			io.Discard, io.Discard, io.Discard, io.Discard,
		).Write(ps5098Payload)
	}
}
