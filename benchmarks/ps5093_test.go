package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5093Input = strings.Repeat("ephemeral-reader-size-", 2978)
	ps5093Size  int64
)

func BenchmarkPS5093_Before(b *testing.B) {
	for b.Loop() {
		ps5093Size = strings.NewReader(strings.Clone(ps5093Input)).Size()
	}
}

func BenchmarkPS5093_After(b *testing.B) {
	for b.Loop() {
		ps5093Size = int64(len(ps5093Input))
	}
}
