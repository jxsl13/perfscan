package benchmarks

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

// The slice is mutated each iteration so the conversion's copy cannot be elided.
var ps5067B = bytes.Repeat([]byte("日本語テキスト"), 3)
var ps5067R bool

// BenchmarkPS5067Before is utf8.FullRuneInString(string(b)): the whole []byte is
// copied into a string just to test one rune.
func BenchmarkPS5067Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5067B[0] = byte(i)
		ps5067R = utf8.FullRuneInString(string(ps5067B))
	}
}

// BenchmarkPS5067After is utf8.FullRune(b): the bytes are tested in place.
func BenchmarkPS5067After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5067B[0] = byte(i)
		ps5067R = utf8.FullRune(ps5067B)
	}
}
