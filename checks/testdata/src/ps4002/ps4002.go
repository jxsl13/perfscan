package ps4002

import "math"

func vsiluF32(dst, src []float32) {}

func silu(dtype int, dst64, src64 []float64, dst32, src32 []float32) {
	switch dtype {
	case 32:
		vsiluF32(dst32, src32)
	case 64:
		for i := range src64 {
			dst64[i] = src64[i] / (1 + math.Exp(-src64[i])) // want `scalar math\.Exp per element in a function that already calls a vectorized sibling kernel — this branch is a proven SIMD candidate; gate the new kernel like the sibling was gated`
		}
	}
}

// No vectorized sibling in this function: no evidence the transform
// applies here, silent.
func plainExp(dst, src []float64) {
	for i := range src {
		dst[i] = math.Exp(src[i])
	}
}
