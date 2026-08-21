package benchmarks

import (
	"path"
	"testing"
)

var (
	ps5109Root = "/srv//perfscan"
	ps5109A    = "artifacts"
	ps5109B    = "../cache"
	ps5109C    = "checks/./PS5109.md"
	ps5109Sink string
)

func BenchmarkPS5109_Before(b *testing.B) {
	for b.Loop() {
		ps5109Sink = path.Join(path.Join(path.Join(ps5109Root, ps5109A), ps5109B), ps5109C)
	}
}

func BenchmarkPS5109_After(b *testing.B) {
	for b.Loop() {
		ps5109Sink = path.Join(ps5109Root, ps5109A, ps5109B, ps5109C)
	}
}
