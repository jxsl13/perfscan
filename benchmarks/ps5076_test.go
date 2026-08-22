package benchmarks

import (
	"io"
	"testing"
)

type ps5076ChunkReader struct{ remaining int }

func (r *ps5076ChunkReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.remaining, 32)
	for i := range n {
		p[i] = byte(i)
	}
	r.remaining -= n
	return n, nil
}

var (
	ps5076N     int64
	ps5076Bytes []byte
)

func BenchmarkPS5076ReadAll_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		r := &ps5076ChunkReader{remaining: 64 << 10}
		ps5076Bytes, _ = io.ReadAll(io.NopCloser(r))
	}
}

func BenchmarkPS5076ReadAll_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		r := &ps5076ChunkReader{remaining: 64 << 10}
		ps5076Bytes, _ = io.ReadAll(r)
	}
}

func BenchmarkPS5076Copy_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		r := &ps5076ChunkReader{remaining: 64 << 10}
		ps5076N, _ = io.Copy(io.Discard, io.NopCloser(r))
	}
}

func BenchmarkPS5076Copy_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		r := &ps5076ChunkReader{remaining: 64 << 10}
		ps5076N, _ = io.Copy(io.Discard, r)
	}
}
