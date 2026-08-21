package benchmarks

import (
	"errors"
	"testing"
)

var (
	errPS5107A      = errors.New("a")
	errPS5107B      = errors.New("b")
	errPS5107C      = errors.New("c")
	errPS5107D      = errors.New("d")
	errPS5107Target = errors.New("missing")
	ps5107Sink      bool
)

func BenchmarkPS5107_Before(b *testing.B) {
	for b.Loop() {
		ps5107Sink = errors.Is(
			errors.Join(errors.Join(errPS5107A, errPS5107B), errors.Join(errPS5107C, errPS5107D)),
			errPS5107Target,
		)
	}
}

func BenchmarkPS5107_After(b *testing.B) {
	for b.Loop() {
		ps5107Sink = errors.Is(
			errors.Join(errPS5107A, errPS5107B, errPS5107C, errPS5107D),
			errPS5107Target,
		)
	}
}
