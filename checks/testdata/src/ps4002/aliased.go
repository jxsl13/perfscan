package ps4002

import m "math"

// The stdlib math imported under an alias still resolves to the same
// package: with the configured vectorized sibling kernel (vsiluF32)
// already called in the same function, the aliased scalar
// transcendental must be flagged too.
func aliasedSilu(dtype int, dst64, src64 []float64, dst32, src32 []float32) {
	switch dtype {
	case 32:
		vsiluF32(dst32, src32)
	case 64:
		for i := range src64 {
			dst64[i] = src64[i] / (1 + m.Exp(-src64[i])) // want `scalar math\.Exp per element in a function that already calls a vectorized sibling kernel — this branch is a proven SIMD candidate; gate the new kernel like the sibling was gated`
		}
	}
}

// NEGATIVE: no vectorized sibling in this function, alias or not: silent.
func aliasedPlainExp(dst, src []float64) {
	for i := range src {
		dst[i] = m.Exp(src[i])
	}
}
