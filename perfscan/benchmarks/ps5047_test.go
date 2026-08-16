package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// BenchmarkPS5047Before is the fmt.Appendf(buf, "%b", n) form the check flags.
func BenchmarkPS5047Before(b *testing.B) {
	buf := make([]byte, 0, 32)
	n := 1234567
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "%b", n)
	}
	_ = buf
}

// BenchmarkPS5047After is the strconv.AppendInt(buf, int64(n), 2) rewrite.
func BenchmarkPS5047After(b *testing.B) {
	buf := make([]byte, 0, 32)
	n := 1234567
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = strconv.AppendInt(buf[:0], int64(n), 2)
	}
	_ = buf
}
