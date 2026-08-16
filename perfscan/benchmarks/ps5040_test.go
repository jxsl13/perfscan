package benchmarks

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

// BenchmarkPS5040Before is the fmt.Appendf(buf, "%c", r) form the check
// flags: fmt parses the format and boxes the rune into an interface.
func BenchmarkPS5040Before(b *testing.B) {
	buf := make([]byte, 0, 8)
	r := rune('世')
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "%c", r)
	}
	_ = buf
}

// BenchmarkPS5040After is the utf8.AppendRune(buf, r) rewrite: the encoder
// runs straight into buf with no format parse and no boxing.
func BenchmarkPS5040After(b *testing.B) {
	buf := make([]byte, 0, 8)
	r := rune('世')
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = utf8.AppendRune(buf[:0], r)
	}
	_ = buf
}
