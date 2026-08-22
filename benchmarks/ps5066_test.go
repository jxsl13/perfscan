package benchmarks

import (
	"bytes"
	"testing"
)

func ps5066Buf() *bytes.Buffer {
	var b bytes.Buffer
	b.Write(make([]byte, 1024))
	return &b
}

var ps5066Sink byte

// BenchmarkPS5066Before is buf.String()[i]: the whole buffer is copied into a
// string just to read one byte.
func BenchmarkPS5066Before(b *testing.B) {
	buf := ps5066Buf()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5066Sink = buf.String()[100]
	}
}

// BenchmarkPS5066After is buf.Bytes()[i]: the byte is read from the backing
// array with no copy.
func BenchmarkPS5066After(b *testing.B) {
	buf := ps5066Buf()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ps5066Sink = buf.Bytes()[100]
	}
}
