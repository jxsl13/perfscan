package benchmarks

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

// BenchmarkPS5061Before is fmt.Appendf(buf, "[%c]", r): a format parse and an
// interface box to UTF-8-encode one rune between two literal runs.
func BenchmarkPS5061Before(b *testing.B) {
	buf := make([]byte, 0, 16)
	r := '世'
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "[%c]", r)
	}
	_ = buf
}

// BenchmarkPS5061After is the nested append/utf8.AppendRune chain rewrite.
func BenchmarkPS5061After(b *testing.B) {
	buf := make([]byte, 0, 16)
	r := '世'
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = append(utf8.AppendRune(append(buf[:0], "["...), r), "]"...)
	}
	_ = buf
}
