package ps6006silent

import "testing"

var useSplitK bool

func BenchmarkDefaultToggle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		useSplitK = i%2 == 0
	}
}
