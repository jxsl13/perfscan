package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// BenchmarkPS5043Before is the fmt.Appendf(buf, "%x", n) form the check flags.
func BenchmarkPS5043Before(b *testing.B) {
	buf := make([]byte, 0, 32)
	n := 1234567
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "%x", n)
	}
	_ = buf
}

// BenchmarkPS5043After is the strconv.AppendInt(buf, int64(n), 16) rewrite.
func BenchmarkPS5043After(b *testing.B) {
	buf := make([]byte, 0, 32)
	n := 1234567
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = strconv.AppendInt(buf[:0], int64(n), 16)
	}
	_ = buf
}
