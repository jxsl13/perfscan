package benchmarks

import (
	"fmt"
	"testing"
)

// BenchmarkPS5049Before is the append(dst, fmt.Sprintf(...)...) form the check
// flags: fmt.Sprintf allocates an intermediate string that append then copies.
func BenchmarkPS5049Before(b *testing.B) {
	buf := make([]byte, 0, 64)
	n, s := 1234567, "hello"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = append(buf[:0], fmt.Sprintf("%d-%s", n, s)...)
	}
	_ = buf
}

// BenchmarkPS5049After is the fmt.Appendf(dst, ...) rewrite: the bytes are
// formatted straight into dst, so the intermediate string never exists.
func BenchmarkPS5049After(b *testing.B) {
	buf := make([]byte, 0, 64)
	n, s := 1234567, "hello"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "%d-%s", n, s)
	}
	_ = buf
}
