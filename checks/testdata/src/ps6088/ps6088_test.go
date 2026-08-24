//go:build go1.22

package ps6088

import "testing"

func TestCallsDoNotEstablishProductionRepetition(t *testing.T) {
	t.Parallel()
	testCallsOnly(1, consume)
	testCallsOnly(2, consume)
}

func BenchmarkOnlyOne(b *testing.B) {
	for b.Loop() {
		benchmarkOnly(1, consume)
	}
}

func BenchmarkOnlyTwo(b *testing.B) {
	for b.Loop() {
		benchmarkOnly(2, consume)
	}
}
