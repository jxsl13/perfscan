package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5124Value = strings.Repeat("field", 4096) + ":" + strings.Repeat("tail", 32)
	ps5124Sink  int
)

func BenchmarkPS5124Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		count := 0
		if strings.Contains(ps5124Value, ":") {
			count = strings.Count(ps5124Value, ":")
		}
		ps5124Sink = count
	}
}

func BenchmarkPS5124After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ps5124Sink = strings.Count(ps5124Value, ":")
	}
}
