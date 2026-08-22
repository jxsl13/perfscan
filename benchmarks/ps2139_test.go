package benchmarks

import (
	"bytes"
	"testing"
)

// PS2139 — WriteString(string(r)) vs WriteRune(r) for a non-constant rune.
// The mixed 1-4 byte rune set exercises every UTF-8 encoding width; the
// before form pays the encode-into-a-fresh-string + copy round trip that
// WriteRune performs directly into the buffer.
var runesMixed = func() []rune {
	out := make([]rune, n)
	shapes := []rune{'a', 'é', '日', '🚀'}
	for i := range out {
		out[i] = shapes[i%len(shapes)]
	}
	return out
}()

func BenchmarkPS2139_Before(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	buf.Grow(4 * n)
	for range b.N {
		buf.Reset()
		for _, r := range runesMixed {
			buf.WriteString(string(r))
		}
		sinkI = buf.Len()
	}
}

func BenchmarkPS2139_After(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	buf.Grow(4 * n)
	for range b.N {
		buf.Reset()
		for _, r := range runesMixed {
			buf.WriteRune(r)
		}
		sinkI = buf.Len()
	}
}
