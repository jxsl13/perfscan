package benchmarks

import (
	"bytes"
	"testing"
)

var ps5062In = bytes.Repeat([]byte("héllo wörld €"), 8)
var ps5062Sink rune

// BenchmarkPS5062Before ranges bytes.Runes(b): a throwaway []rune allocation.
func BenchmarkPS5062Before(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, r := range bytes.Runes(ps5062In) {
			ps5062Sink = r
		}
	}
}

// BenchmarkPS5062After ranges string(b): the runes are decoded in place, no
// []rune allocation.
func BenchmarkPS5062After(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, r := range string(ps5062In) {
			ps5062Sink = r
		}
	}
}
