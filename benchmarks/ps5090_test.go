package benchmarks

import (
	"strconv"
	"strings"
	"testing"
)

var (
	ps5090Input = string(ps5086Input)
	ps5090Text  string
)

func BenchmarkPS5090_Before(b *testing.B) {
	for b.Loop() {
		ps5090Text = strconv.Quote(strings.Clone(ps5090Input))
	}
}

func BenchmarkPS5090_After(b *testing.B) {
	for b.Loop() {
		ps5090Text = strconv.Quote(ps5090Input)
	}
}
