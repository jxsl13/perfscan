package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5122Value = strings.Repeat("field", 4096) + ":" + strings.Repeat("tail", 32)
	ps5122Sink  string
)

func BenchmarkPS5122Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		value := ps5122Value
		if strings.Contains(value, ":") {
			value = strings.ReplaceAll(value, ":", "-")
		}
		ps5122Sink = value
	}
}

func BenchmarkPS5122After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		value := ps5122Value
		value = strings.ReplaceAll(value, ":", "-")
		ps5122Sink = value
	}
}
