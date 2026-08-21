package ps6060native

// The active package already exposes a native SIMD sibling for this dtype, so
// its scalar tail/fallback should not receive a duplicate recommendation.
func reluF32NEON(dst, src []float32) {}

func scalarFallback(dst, src []float32) {
	for i := range dst {
		if src[i] > 0 {
			dst[i] = src[i]
		}
	}
}

// The float32 sibling must not suppress a distinct float64 opportunity.
func scalarF64(dst, src []float64) {
	for i := range dst { // want `exact float64 ReLU loop src\[i\] > 0 -> dst\[i\] remains scalar`
		if src[i] > 0 {
			dst[i] = src[i]
		}
	}
}

var _ = []any{reluF32NEON, scalarFallback, scalarF64}
