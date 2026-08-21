package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5126Value = strings.Repeat("field", 4096) + ":" + strings.Repeat("tail", 32)
	ps5126Sink  int
)

func BenchmarkPS5126Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		index := -1
		if strings.Contains(ps5126Value, ":") {
			index = strings.LastIndex(ps5126Value, ":")
		}
		ps5126Sink = index
	}
}

func BenchmarkPS5126After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ps5126Sink = strings.LastIndex(ps5126Value, ":")
	}
}
