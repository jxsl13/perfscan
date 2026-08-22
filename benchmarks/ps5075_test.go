package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5075Input = strings.Repeat("PERFSCAN", 8*1024) // 64 KiB
	ps5075Sink  string
)

func BenchmarkPS5075_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5075Sink = strings.ToLower(strings.ToLower(ps5075Input))
	}
}

func BenchmarkPS5075_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5075Sink = strings.ToLower(ps5075Input)
	}
}
