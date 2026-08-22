package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// BenchmarkPS5060Before is fmt.Appendf(buf, "k=%q;", s): a format parse and an
// interface box to splice one quoted string between two literal runs.
func BenchmarkPS5060Before(b *testing.B) {
	buf := make([]byte, 0, 32)
	s := "value"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "k=%q;", s)
	}
	_ = buf
}

// BenchmarkPS5060After is the nested append/strconv.AppendQuote chain rewrite.
func BenchmarkPS5060After(b *testing.B) {
	buf := make([]byte, 0, 32)
	s := "value"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = append(strconv.AppendQuote(append(buf[:0], "k="...), s), ";"...)
	}
	_ = buf
}
