package benchmarks

import (
	"bytes"
	"regexp"
	"testing"
)

var (
	ps5088Regexp = regexp.MustCompile(`not-present$`)
	ps5088Match  bool
)

func BenchmarkPS5088_Before(b *testing.B) {
	for b.Loop() {
		ps5088Match = ps5088Regexp.Match(bytes.Clone(ps5086Input))
	}
}

func BenchmarkPS5088_After(b *testing.B) {
	for b.Loop() {
		ps5088Match = ps5088Regexp.Match(ps5086Input)
	}
}
