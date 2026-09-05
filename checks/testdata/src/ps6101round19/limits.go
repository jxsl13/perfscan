package ps6101round19

import "testing"

var pathFlags [9]bool

func deferredPathLimit(weights []float64) {
	if pathFlags[0] {
		defer random(weights)
	}
	if pathFlags[1] {
		defer random(weights)
	}
	if pathFlags[2] {
		defer random(weights)
	}
	if pathFlags[3] {
		defer random(weights)
	}
	if pathFlags[4] {
		defer random(weights)
	}
	if pathFlags[5] {
		defer random(weights)
	}
	if pathFlags[6] {
		defer random(weights)
	}
	if pathFlags[7] {
		defer random(weights)
	}
	if pathFlags[8] {
		defer random(weights)
	}
}

func exceedFreshLimit(weight float64) *float64 {
	var result *float64
	for index := 0; index < 257; index++ {
		result = new(float64)
	}
	*result = weight
	return result
}

func BenchmarkDeferredPathLimitOpaque(b *testing.B) {
	weights := []float64{1}
	deferredPathLimit(weights)
	safeGate(b, weights[0])
}

func BenchmarkFreshLimitOpaque(b *testing.B) {
	weights := []float64{1}
	weight := exceedFreshLimit(weights[0])
	safeGate(b, *weight)
}
