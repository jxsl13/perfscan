package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// BenchmarkPS5041Before is the fmt.Appendf(buf, "%q", s) form the check flags.
func BenchmarkPS5041Before(b *testing.B) {
	buf := make([]byte, 0, 64)
	s := "cache-key-42"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "%q", s)
	}
	_ = buf
}

// BenchmarkPS5041After is the strconv.AppendQuote(buf, s) rewrite.
func BenchmarkPS5041After(b *testing.B) {
	buf := make([]byte, 0, 64)
	s := "cache-key-42"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = strconv.AppendQuote(buf[:0], s)
	}
	_ = buf
}
