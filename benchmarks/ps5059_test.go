package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// BenchmarkPS5059Before is fmt.Appendf(buf, "id=%d;", n): a format parse and an
// interface box to splice one integer between two literal runs.
func BenchmarkPS5059Before(b *testing.B) {
	buf := make([]byte, 0, 32)
	n := 1234567
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "id=%d;", n)
	}
	_ = buf
}

// BenchmarkPS5059After is the nested append/strconv.AppendInt chain rewrite.
func BenchmarkPS5059After(b *testing.B) {
	buf := make([]byte, 0, 32)
	n := 1234567
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = append(strconv.AppendInt(append(buf[:0], "id="...), int64(n), 10), ";"...)
	}
	_ = buf
}
