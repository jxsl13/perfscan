package ps6063native

func negScalarF32(dst, src []float32) {
	for i := range dst {
		dst[i] = -src[i]
	}
}

func negF32NEON(dst, src []float32)

// The float32 native sibling does not suppress a float64 candidate.
func negScalarF64(dst, src []float64) {
	for i := range dst { // want `exact float64 negation loop src\[i\] -> -src\[i\] -> dst\[i\] remains scalar`
		dst[i] = -src[i]
	}
}

var _ = []any{negScalarF32, negScalarF64}
