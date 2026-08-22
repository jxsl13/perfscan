package benchmarks

import (
	"strconv"
	"testing"
)

// PS2136 — []byte(strconv.Itoa(n)) vs strconv.AppendInt(nil, int64(n), 10):
// the After skips the intermediate string allocation and its copy.

var ps2136Sink []byte

func BenchmarkPS2136_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2136Sink = []byte(strconv.Itoa(123456))
	}
}

func BenchmarkPS2136_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2136Sink = strconv.AppendInt(nil, int64(123456), 10)
	}
}
