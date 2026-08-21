package ps6014alias

import tt "testing"

func BenchmarkCUDAGroupedDispatchLatencyFusion(b *tt.B) { // want "structural accelerator-fusion benchmark has no separate structural/latency leverage manifest"
	for range b.N {
	}
}
