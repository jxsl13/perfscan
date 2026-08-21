package benchmarks

import (
	"io"
	"testing"
)

type ps5096Reader struct{}

func (ps5096Reader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	buffer[0] = 1
	return 1, nil
}

var (
	ps5096Input  io.Reader = ps5096Reader{}
	ps5096Buffer           = make([]byte, 1)
	ps5096Inner  int64     = 128
	ps5096Middle int64     = 64
	ps5096Outer  int64     = 32
	ps5096N      int
	ps5096Err    error
)

func BenchmarkPS5096_Before(b *testing.B) {
	for b.Loop() {
		ps5096N, ps5096Err = io.LimitReader(
			io.LimitReader(io.LimitReader(ps5096Input, ps5096Inner), ps5096Middle),
			ps5096Outer,
		).Read(ps5096Buffer)
	}
}

func BenchmarkPS5096_After(b *testing.B) {
	for b.Loop() {
		ps5096N, ps5096Err = io.LimitReader(
			ps5096Input,
			min(ps5096Inner, ps5096Middle, ps5096Outer),
		).Read(ps5096Buffer)
	}
}
