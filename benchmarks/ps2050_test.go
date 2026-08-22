package benchmarks

import (
	"testing"
	"unicode/utf8"
)

var sinkPS2050 string

// BenchmarkPS2050Before is the string(utf8.AppendRune(nil, r)) form the check
// flags: a throwaway []byte allocation plus the string copy.
func BenchmarkPS2050Before(b *testing.B) {
	r := rune('世')
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS2050 = string(utf8.AppendRune(nil, r))
	}
}

// BenchmarkPS2050After is the string(r) rewrite: one step, no throwaway slice.
func BenchmarkPS2050After(b *testing.B) {
	r := rune('世')
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkPS2050 = string(r)
	}
}
