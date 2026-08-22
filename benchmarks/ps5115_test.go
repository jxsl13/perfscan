package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5115Input = strings.Repeat("A\xff", 48*1024)
	ps5115Sink  string
)

func BenchmarkPS5115_Before(b *testing.B) {
	for b.Loop() {
		ps5115Sink = strings.ToValidUTF8(strings.ToValidUTF8(ps5115Input, "�"), "\xff")
	}
}

func BenchmarkPS5115_After(b *testing.B) {
	for b.Loop() {
		ps5115Sink = strings.ToValidUTF8(ps5115Input, "�")
	}
}
