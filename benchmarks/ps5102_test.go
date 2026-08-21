package benchmarks

import (
	"bytes"
	"testing"
)

// PS5102 — WriteRune of a constant single-byte rune vs WriteByte. The std
// writers fast-path runes below utf8.RuneSelf inside WriteRune, so the
// difference is the skipped wrapper: the encoder branch and the unused
// (int, error) result.
func BenchmarkPS5102_Before(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	buf.Grow(n)
	for range b.N {
		buf.Reset()
		for range n {
			buf.WriteRune(',')
		}
		sinkI = buf.Len()
	}
}

func BenchmarkPS5102_After(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	buf.Grow(n)
	for range b.N {
		buf.Reset()
		for range n {
			buf.WriteByte(',')
		}
		sinkI = buf.Len()
	}
}
