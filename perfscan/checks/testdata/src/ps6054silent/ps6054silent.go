package ps6054silent

import "testing"

func scalarAttentionKernel()  {}
func stripedAttentionKernel() {}

func selectAttention(contextLength int) {
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
