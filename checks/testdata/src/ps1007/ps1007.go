package ps1007

func axpyRestream(out, w, in []float64, n, dim int) {
	for i := 0; i < n; i++ {
		v := w[i]
		for d := 0; d < dim; d++ {
			out[d] += v * in[i*dim+d] // want `this inner loop accumulates into an output row invariant in outer variable i — all of it is loaded and stored once per outer step; the remedy depends on whether the input is contiguous in d \(strip-mine the gather, outer-unroll the rank-1 update, register-tile a band\) — see PS1007 docs`
		}
	}
}

func directOuter(out, in []float64, n, dim int) {
	for i := 0; i < n; i++ {
		for d := 0; d < dim; d++ {
			out[d] += in[i*dim+d] // want `this inner loop accumulates into an output row invariant in outer variable i — all of it is loaded and stored once per outer step; the remedy depends on whether the input is contiguous in d \(strip-mine the gather, outer-unroll the rank-1 update, register-tile a band\) — see PS1007 docs`
		}
	}
}

// The output row varies with the outer variable: each row written once, no
// restreaming — silent.
func perRow(out [][]float64, in []float64, n, dim int) {
	for i := 0; i < n; i++ {
		for d := 0; d < dim; d++ {
			out[i][d] += in[i*dim+d]
		}
	}
}

// The accumulated term does not vary with the outer iteration: a repeated
// constant add, not this pattern — silent.
func constantAdd(out []float64, n, dim int) {
	for i := 0; i < n; i++ {
		for d := 0; d < dim; d++ {
			out[d] += 1
		}
	}
}

// Scalar reduction target is not an output row: silent.
func scalarSum(in []float64, n, dim int) float64 {
	s := 0.0
	for i := 0; i < n; i++ {
		for d := 0; d < dim; d++ {
			s += in[i*dim+d]
		}
	}
	return s
}
