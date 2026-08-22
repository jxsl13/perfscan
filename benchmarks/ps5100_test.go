package benchmarks

import (
	"io"
	"testing"
)

type ps5100EOFReader struct{}

func (ps5100EOFReader) Read([]byte) (int, error) { return 0, io.EOF }

var (
	ps5100Reader io.Reader = ps5100EOFReader{}
	ps5100N      int64
	ps5100Err    error
)

func BenchmarkPS5100_Before(b *testing.B) {
	for b.Loop() {
		ps5100N, ps5100Err = io.Copy(io.Discard,
			io.MultiReader(
				io.MultiReader(ps5100Reader, ps5100Reader),
				io.MultiReader(ps5100Reader, ps5100Reader),
			),
		)
	}
}

func BenchmarkPS5100_After(b *testing.B) {
	for b.Loop() {
		ps5100N, ps5100Err = io.Copy(io.Discard,
			io.MultiReader(ps5100Reader, ps5100Reader, ps5100Reader, ps5100Reader),
		)
	}
}
