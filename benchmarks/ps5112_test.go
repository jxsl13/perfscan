package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5112Input = strings.Repeat("field-0123456789/", 4096)
	ps5112Sink  string
)

func BenchmarkPS5112_Before(b *testing.B) {
	for b.Loop() {
		ps5112Sink = strings.Join(strings.Split(ps5112Input, "/"), "/")
	}
}

func BenchmarkPS5112_After(b *testing.B) {
	for b.Loop() {
		ps5112Sink = ps5112Input
	}
}
