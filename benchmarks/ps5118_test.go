package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5118Input = strings.Repeat("alpha_beta_test\n", 6*1024)
	ps5118Sink  string
)

func BenchmarkPS5118_Before(b *testing.B) {
	for b.Loop() {
		ps5118Sink = strings.ReplaceAll(strings.ReplaceAll(ps5118Input, "\x00", "_"), "\x00", "unused")
	}
}

func BenchmarkPS5118_After(b *testing.B) {
	for b.Loop() {
		ps5118Sink = strings.ReplaceAll(ps5118Input, "\x00", "_")
	}
}
