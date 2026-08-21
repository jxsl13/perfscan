package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5091Input = strings.Repeat("clone-index-payload-", 3449)
	ps5091Byte  byte
)

func BenchmarkPS5091_Before(b *testing.B) {
	for b.Loop() {
		ps5091Byte = strings.Clone(ps5091Input)[len(ps5091Input)-1]
	}
}

func BenchmarkPS5091_After(b *testing.B) {
	for b.Loop() {
		ps5091Byte = ps5091Input[len(ps5091Input)-1]
	}
}
