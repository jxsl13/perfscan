package ps3034

func parallelFor(n int, f func(int)) {
	for i := 0; i < n; i++ {
		f(i)
	}
}

func transform(v float64) float64 { return v * 2 }

// The package declares parallelFor; this function never calls it, and
// every write names the outer variable b: bandable.
func serialNest(dst, src [][]float64, batch, d int) {
	for b := 0; b < batch; b++ { // want `every write in this serial nest names outer variable b — the outer iterations own disjoint output and a fan-out helper exists in this package; band the outer loop and gate with BOTH a bit-exact digest and -race`
		for j := 0; j < d; j++ {
			dst[b][j] = transform(src[b][j])
		}
	}
}

// Writes through a derived base do not name b directly: PS3059's
// territory, this check is silent.
func derivedBase(dst, src []float64, batch, out int) {
	for b := 0; b < batch; b++ {
		obase := b * out
		for j := 0; j < out; j++ {
			dst[obase+j] = src[j]
		}
	}
}

// A shared accumulator does not name the outer variable: not disjoint,
// silent.
func sharedAccum(m [][]float64, x []float64, sum []float64) {
	for i := range x {
		for j := range x {
			sum[j] += m[i][j]
		}
	}
}

// The function fans out already: silent.
func banded(dst, src [][]float64, batch, d int) {
	parallelFor(batch, func(b int) {
		for j := 0; j < d; j++ {
			dst[b][j] = transform(src[b][j])
		}
	})
}
