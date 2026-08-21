package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5121Value = strings.Repeat("field", 32) + ":" + strings.Repeat("tail", 16)
	ps5121Sink  string
)

func BenchmarkPS5121Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		value := ps5121Value
		if strings.Contains(value, ":") {
			value = strings.SplitN(value, ":", 2)[1]
		}
		ps5121Sink = value
	}
}

func BenchmarkPS5121After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		value := ps5121Value
		if _, after, found := strings.Cut(value, ":"); found {
			value = after
		}
		ps5121Sink = value
	}
}
