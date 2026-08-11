package benchmarks

import (
	"fmt"
	"testing"
)

// PS2109 — []byte(fmt.Sprintf(...)) vs fmt.Appendf(nil, ...): the After
// skips the intermediate string allocation and its copy.

var ps2109Sink []byte

func BenchmarkPS2109_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2109Sink = []byte(fmt.Sprintf("user=%d flags=%s", 123456, "rw"))
	}
}

func BenchmarkPS2109_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2109Sink = fmt.Appendf(nil, "user=%d flags=%s", 123456, "rw")
	}
}
