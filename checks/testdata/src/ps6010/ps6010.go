package ps6010

func matvec(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
}

// Bias-seeded accumulator: still reported, but the seed is not the canonical
// 0.0 literal, so no automatic rewrite is offered.
func matvecBias(dst, a, w, bias []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := bias[o]
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
}

// Reordered weight index: still reported, but the index is not the canonical
// i*out+o shape, so no automatic rewrite is offered.
func matvecSwapped(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o+i*out] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
}

// Both factors vary with the output index: nothing is reloaded invariant.
func hadamard(dst, a, b []float64, n int) {
	for o := 0; o < n; o++ {
		acc := 0.0
		for i := 0; i < 1; i++ {
			acc += a[o] * b[o]
		}
		dst[o] = acc
	}
}

// No enclosing output loop: a single dot product has nothing to amortize
// across.
func dot(a, b []float64) float64 {
	acc := 0.0
	for i := range a {
		acc += a[i] * b[i]
	}
	return acc
}
