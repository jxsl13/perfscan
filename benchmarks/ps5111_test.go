package benchmarks

import (
	"path"
	"strings"
	"testing"
)

var (
	ps5111Input = "/srv/" + strings.Repeat("canonical-segment/", 4096) + "leaf"
	ps5111Sink  string
)

func BenchmarkPS5111_Before(b *testing.B) {
	for b.Loop() {
		ps5111Sink = path.Clean(path.Dir(ps5111Input))
	}
}

func BenchmarkPS5111_After(b *testing.B) {
	for b.Loop() {
		ps5111Sink = path.Dir(ps5111Input)
	}
}
