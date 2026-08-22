package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// BenchmarkPS5042Before is the fmt.Appendf(buf, "%d", n) form the check flags.
func BenchmarkPS5042Before(b *testing.B) {
	buf := make([]byte, 0, 32)
	n := 1234567
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "%d", n)
	}
	_ = buf
}

// BenchmarkPS5042After is the strconv.AppendInt(buf, int64(n), 10) rewrite.
func BenchmarkPS5042After(b *testing.B) {
	buf := make([]byte, 0, 32)
	n := 1234567
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = strconv.AppendInt(buf[:0], int64(n), 10)
	}
	_ = buf
}
