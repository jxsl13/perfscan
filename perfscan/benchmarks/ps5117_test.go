package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5117Input = strings.Repeat(" alpha\tbeta\ngamma  ", 5*1024)
	ps5117Sink  string
)

func BenchmarkPS5117_Before(b *testing.B) {
	for b.Loop() {
		ps5117Sink = strings.Join(strings.Fields(strings.Join(strings.Fields(ps5117Input), " ")), " ")
	}
}

func BenchmarkPS5117_After(b *testing.B) {
	for b.Loop() {
		ps5117Sink = strings.Join(strings.Fields(ps5117Input), " ")
	}
}
