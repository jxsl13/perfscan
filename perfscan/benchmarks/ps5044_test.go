package benchmarks

import (
	"fmt"
	"testing"
)

// BenchmarkPS5044Before is the fmt.Appendf(buf, "%v", s) form the check flags.
func BenchmarkPS5044Before(b *testing.B) {
	buf := make([]byte, 0, 64)
	s := "the quick brown fox jumps"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "%v", s)
	}
	_ = buf
}

// BenchmarkPS5044After is the append(buf, s...) rewrite.
func BenchmarkPS5044After(b *testing.B) {
	buf := make([]byte, 0, 64)
	s := "the quick brown fox jumps"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = append(buf[:0], s...)
	}
	_ = buf
}
