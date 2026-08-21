package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// BenchmarkPS5068Before is fmt.Appendf(buf, "%f", f): a format parse and an
// interface box to print one float.
func BenchmarkPS5068Before(b *testing.B) {
	buf := make([]byte, 0, 32)
	f := 3.14159265358979
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = fmt.Appendf(buf[:0], "%f", f)
	}
	_ = buf
}

// BenchmarkPS5068After is the strconv.AppendFloat(buf, f, 'f', 6, 64) rewrite.
func BenchmarkPS5068After(b *testing.B) {
	buf := make([]byte, 0, 32)
	f := 3.14159265358979
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = strconv.AppendFloat(buf[:0], f, 'f', 6, 64)
	}
	_ = buf
}
