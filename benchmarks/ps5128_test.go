package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5128Value = strings.Repeat("field", 4096) + ":" + strings.Repeat("tail", 32)
	ps5128Sink  []string
)

func BenchmarkPS5128Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var parts []string
		if strings.Contains(ps5128Value, ":") {
			parts = strings.Split(ps5128Value, ":")
		} else {
			parts = []string{ps5128Value}
		}
		ps5128Sink = parts
	}
}

func BenchmarkPS5128After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ps5128Sink = strings.Split(ps5128Value, ":")
	}
}
