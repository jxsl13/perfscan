package benchmarks

import "testing"

// PS3090 — a capturing closure dispatched to a worker pool vs a typed,
// non-capturing task payload. When the task value escapes (a persistent pool
// stores it in a channel), a fresh closure capturing the operation's slices is
// heap-allocated on every dispatch, while a typed struct payload carrying the
// same slice headers is not. Modeled here by letting each task escape to a
// package-level sink.

type ps3090Task struct{ dst, src []float64 }

var (
	ps3090SinkFn   func(lo, hi int)
	ps3090SinkTask ps3090Task
)

func BenchmarkPS3090_Before(b *testing.B) {
	dst := make([]float64, 4096)
	src := make([]float64, 4096)
	b.ReportAllocs()
	for range b.N {
		// A new closure capturing dst+src is built and escapes to the sink.
		ps3090SinkFn = func(lo, hi int) {
			for i := lo; i < hi; i++ {
				dst[i] = src[i]
			}
		}
	}
	ps3090SinkFn(0, 0)
}

func BenchmarkPS3090_After(b *testing.B) {
	dst := make([]float64, 4096)
	src := make([]float64, 4096)
	b.ReportAllocs()
	for range b.N {
		// A typed payload carries the same slice headers with no closure env.
		ps3090SinkTask = ps3090Task{dst: dst, src: src}
	}
	_ = ps3090SinkTask
}
