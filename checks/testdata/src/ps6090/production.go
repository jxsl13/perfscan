package ps6090

import "testing"

func ProductionCompute() int { return 1 }

func productionBenchmarkHelper(b *testing.B) {
	for b.Loop() {
		ProductionCompute()
	}
}

func BenchmarkInProductionFile(b *testing.B) {
	productionBenchmarkHelper(b)
}

type fakeB struct {
	N int
}

func (*fakeB) Loop() bool { return false }

func BenchmarkWrongParameter(b *fakeB) {
	for b.Loop() {
		ProductionCompute()
	}
}
