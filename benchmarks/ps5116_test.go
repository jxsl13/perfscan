package benchmarks

import (
	"strings"
	"testing"
	"unicode/utf8"
)

var (
	ps5116Input = strings.Repeat("A\xff", 48*1024)
	ps5116Sink  bool
)

func BenchmarkPS5116_Before(b *testing.B) {
	for b.Loop() {
		ps5116Sink = utf8.ValidString(strings.ToValidUTF8(ps5116Input, "�"))
	}
}

func BenchmarkPS5116_After(b *testing.B) {
	for b.Loop() {
		ps5116Sink = true
	}
}
