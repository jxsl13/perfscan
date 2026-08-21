package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5092Left  = strings.Repeat("clone-comparison-payload-", 2621)
	ps5092Right = strings.Clone(ps5092Left)
	ps5092Sink  bool
)

func BenchmarkPS5092_Before(b *testing.B) {
	for b.Loop() {
		ps5092Sink = strings.Clone(strings.Clone(ps5092Left)) == strings.Clone(ps5092Right)
	}
}

func BenchmarkPS5092_After(b *testing.B) {
	for b.Loop() {
		ps5092Sink = ps5092Left == ps5092Right
	}
}
