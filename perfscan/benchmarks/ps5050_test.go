package benchmarks

import (
	"bytes"
	"slices"
	"testing"
)

var ps5050Haystack = bytes.Repeat([]byte("abcdefghijklmnop"), 64) // 1 KB, target absent

// BenchmarkPS5050Before is the slices.Index(b, c) form the check flags: the
// generic element-by-element scan.
func BenchmarkPS5050Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = slices.Index(ps5050Haystack, byte('z'))
	}
}

// BenchmarkPS5050After is the bytes.IndexByte(b, c) rewrite: the SIMD byte
// search.
func BenchmarkPS5050After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bytes.IndexByte(ps5050Haystack, 'z')
	}
}
