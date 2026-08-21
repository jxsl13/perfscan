package benchmarks

import (
	"path"
	"strings"
	"testing"
)

var (
	ps5082Left  = strings.Repeat("clone-observer-payload-", 2849)
	ps5082Right = strings.Clone(ps5082Left)
	ps5082Sink  int
	ps5082Bool  bool
)

func BenchmarkPS5082_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5082Sink = strings.Compare(
			strings.Clone(strings.Clone(ps5082Left)),
			strings.Clone(strings.Clone(ps5082Right)),
		)
	}
}

func BenchmarkPS5082IsAbs_Before(b *testing.B) {
	for b.Loop() {
		ps5082Bool = path.IsAbs(strings.Clone(ps5082Left))
	}
}

func BenchmarkPS5082IsAbs_After(b *testing.B) {
	for b.Loop() {
		ps5082Bool = path.IsAbs(ps5082Left)
	}
}

func BenchmarkPS5082_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5082Sink = strings.Compare(ps5082Left, ps5082Right)
	}
}
