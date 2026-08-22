package benchmarks

import (
	"strings"
	"testing"
)

// PS2128 — string += in a loop vs strings.Builder over the same 1024
// words. The += loop allocates a new string and re-copies the whole
// accumulated prefix on every iteration (quadratic bytes copied); the
// builder appends into an amortized-growth buffer and converts once.
func BenchmarkPS2128_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		acc := ""
		for i := range words {
			acc += words[i]
		}
		sinkS = acc
	}
}

func BenchmarkPS2128_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		var acc strings.Builder
		for i := range words {
			acc.WriteString(words[i])
		}
		sinkS = acc.String()
	}
}
