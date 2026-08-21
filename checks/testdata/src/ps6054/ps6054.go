package ps6054

import "testing"

func scalarAttentionKernel()   {}
func stripedAttentionKernel()  {}
func fastAttentionKernel()     {}
func fallbackAttentionKernel() {}
func zeroOutput()              {}
func decodeReference()         {}

func selectAttention(contextLength int) { // want `leaf-only shape selector selectAttention is covered by BenchmarkAttentionLeaf but has no complete caller-level promotion contract; missing caller-level benchmark, at least 10 independent samples, order-alternating execution, equivalent-output gate, retained-shape regression bound`
	if contextLength <= 2 {
		scalarAttentionKernel()
	} else {
		stripedAttentionKernel()
	}
}

func BenchmarkAttentionLeaf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		selectAttention(1)
	}
}

type device struct{ supportsFast bool }

// Capability guards are not shape thresholds.
func selectCapability(device device) {
	if device.supportsFast {
		fastAttentionKernel()
	} else {
		fallbackAttentionKernel()
	}
}

func BenchmarkCapabilityLeaf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		selectCapability(device{supportsFast: true})
	}
}

// An empty-input correctness fallback does not select two performance kernels.
func selectCorrectness(values []float32) {
	if len(values) == 0 {
		zeroOutput()
	} else {
		decodeReference()
	}
}

func BenchmarkCorrectnessLeaf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		selectCorrectness(nil)
	}
}

// A source selector without repeated leaf-benchmark coverage is outside this
// rule; other verification can request evidence when it is promoted.
func selectUnbenchmarked(sequenceLength int) {
	if sequenceLength <= 2 {
		scalarAttentionKernel()
	} else {
		stripedAttentionKernel()
	}
}

var _ = selectUnbenchmarked
