package benchmarks

import "testing"

type ps6105BeforeState struct {
	projection   []float32
	accumulation []float32
	mode         int
}

type ps6105AfterState struct {
	mode int
}

var (
	ps6105BeforeSink *ps6105BeforeState
	ps6105AfterSink  *ps6105AfterState
)

//go:noinline
func ps6105Before() *ps6105BeforeState {
	return &ps6105BeforeState{
		projection:   make([]float32, 1024*768),
		accumulation: make([]float32, 1024*768),
		mode:         1,
	}
}

//go:noinline
func ps6105After() *ps6105AfterState {
	return &ps6105AfterState{mode: 1}
}

func BenchmarkPS6105_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps6105BeforeSink = ps6105Before()
	}
}

func BenchmarkPS6105_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps6105AfterSink = ps6105After()
	}
}
