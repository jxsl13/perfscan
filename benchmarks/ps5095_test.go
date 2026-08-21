package benchmarks

import (
	"io"
	"testing"
)

type ps5095Reader struct{}

func (ps5095Reader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	buffer[0] = 1
	return 1, nil
}

var (
	ps5095Input  io.Reader = ps5095Reader{}
	ps5095Buffer           = make([]byte, 1)
	ps5095N      int
	ps5095Err    error
)

func BenchmarkPS5095_Before(b *testing.B) {
	for b.Loop() {
		ps5095N, ps5095Err = io.NopCloser(io.NopCloser(io.NopCloser(ps5095Input))).Read(ps5095Buffer)
	}
}

func BenchmarkPS5095_After(b *testing.B) {
	for b.Loop() {
		ps5095N, ps5095Err = ps5095Input.Read(ps5095Buffer)
	}
}
