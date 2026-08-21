package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5123Value = strings.Repeat("field", 4096) + ":" + strings.Repeat("tail", 32)
	ps5123Sink  int
)

func BenchmarkPS5123Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		index := -1
		if strings.Contains(ps5123Value, ":") {
			index = strings.Index(ps5123Value, ":")
		}
		ps5123Sink = index
	}
}

func BenchmarkPS5123After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ps5123Sink = strings.Index(ps5123Value, ":")
	}
}
