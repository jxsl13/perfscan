package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5120Value = strings.Repeat("field", 16) + ":" + strings.Repeat("tail", 16)
	ps5120Sink  string
)

func BenchmarkPS5120Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		head := strings.SplitN(ps5120Value, ":", 2)[0]
		ps5120Sink = head
	}
}

func BenchmarkPS5120After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		head, _, _ := strings.Cut(ps5120Value, ":")
		ps5120Sink = head
	}
}
