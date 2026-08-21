package benchmarks

import (
	"strings"
	"testing"
	"unicode/utf8"
)

var (
	ps5127Value = strings.Repeat("payload", 14_000) + "\xff"
	ps5127Sink  string
)

func BenchmarkPS5127Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		value := ps5127Value
		if !utf8.ValidString(value) {
			value = strings.ToValidUTF8(value, "?")
		}
		ps5127Sink = value
	}
}

func BenchmarkPS5127After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ps5127Sink = strings.ToValidUTF8(ps5127Value, "?")
	}
}
