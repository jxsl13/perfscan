package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5079Input = "payload"
	ps5079Sink  string
)

func BenchmarkPS5079_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5079Sink = strings.Trim(strings.TrimPrefix(strings.TrimSuffix(strings.TrimLeft(strings.TrimRight(ps5079Input, ""), ""), ""), ""), "")
	}
}

func BenchmarkPS5079_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5079Sink = ps5079Input
	}
}
