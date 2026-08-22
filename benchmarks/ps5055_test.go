package benchmarks

import (
	"bytes"
	"slices"
	"testing"
)

var (
	ps5055A = bytes.Repeat([]byte("perfscan-x"), 128) // 1280 bytes
	ps5055B = append(bytes.Repeat([]byte("perfscan-x"), 128), 'y')
	ps5055E bool
	ps5055C int
)

// BenchmarkPS5055EqualBefore is slices.Equal(a, a): the generic element loop.
func BenchmarkPS5055EqualBefore(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5055E = slices.Equal(ps5055A, ps5055A)
	}
}

// BenchmarkPS5055EqualAfter is bytes.Equal(a, a): the SIMD memequal.
func BenchmarkPS5055EqualAfter(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5055E = bytes.Equal(ps5055A, ps5055A)
	}
}

// BenchmarkPS5055CompareBefore is slices.Compare(a, b): the per-byte loop.
func BenchmarkPS5055CompareBefore(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5055C = slices.Compare(ps5055A, ps5055B)
	}
}

// BenchmarkPS5055CompareAfter is bytes.Compare(a, b): the SIMD memcmp.
func BenchmarkPS5055CompareAfter(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5055C = bytes.Compare(ps5055A, ps5055B)
	}
}
