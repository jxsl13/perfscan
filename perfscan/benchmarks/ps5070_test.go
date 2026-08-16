package benchmarks

import (
	"bytes"
	"io"
	"testing"
)

// PS5070 — w.WriteString(string(b)) vs w.Write(b) on a receiver held
// behind a Writer+StringWriter interface (the shape where the copy always
// happens). The Before side heap-allocates a string copy of each slice
// just to hand the same bytes to WriteString; the After side writes the
// slice directly — allocation-free. 64 slices of ~83 bytes each, the
// shape from the check's MeasuredWin.
type ps5070Writer interface {
	io.Writer
	io.StringWriter
}

var ps5070Payloads = func() [][]byte {
	p := make([][]byte, 64)
	for i := range p {
		p[i] = bytes.Repeat([]byte("x"), 83)
	}
	return p
}()

func BenchmarkPS5070_Before(b *testing.B) {
	var buf bytes.Buffer
	var w ps5070Writer = &buf
	b.ReportAllocs()
	for range b.N {
		buf.Reset()
		for _, p := range ps5070Payloads {
			w.WriteString(string(p))
		}
	}
}

func BenchmarkPS5070_After(b *testing.B) {
	var buf bytes.Buffer
	var w ps5070Writer = &buf
	b.ReportAllocs()
	for range b.N {
		buf.Reset()
		for _, p := range ps5070Payloads {
			w.Write(p)
		}
	}
}
