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

// Un-hoisted variant: the weight is read directly per element — the same
// canonical rank-1 update, rewritten the same way.
func axpyDirect(out, w, in []float64, n, dim int) {
	for i := 0; i < n; i++ {
		for d := 0; d < dim; d++ {
			out[d] += w[i] * in[i*dim+d] // want `this inner loop accumulates into an output row invariant in outer variable i — all of it is loaded and stored once per outer step; the remedy depends on whether the input is contiguous in d \(strip-mine the gather, outer-unroll the rank-1 update, register-tile a band\) — see PS1007 docs`
		}
	}
}

// Non-simple inner bound: still reported, but len(out) is not a plain
// identifier or selector bound the rewrite may repeat, so no automatic fix
// is offered.
func axpyLenBound(out, w, in []float64, n int) {
	for i := 0; i < n; i++ {
		v := w[i]
		for d := 0; d < len(out); d++ {
			out[d] += v * in[i*len(out)+d] // want `this inner loop accumulates into an output row invariant in outer variable i — all of it is loaded and stored once per outer step; the remedy depends on whether the input is contiguous in d \(strip-mine the gather, outer-unroll the rank-1 update, register-tile a band\) — see PS1007 docs`
		}
	}
}

// Extra work in the outer body besides the hoist: still reported, but the
// body is not the exact canonical shape, so no automatic fix is offered.
func axpyScaled(out, w, in []float64, n, dim int) {
	for i := 0; i < n; i++ {
		v := w[i]
		v *= 2
		for d := 0; d < dim; d++ {
			out[d] += v * in[i*dim+d] // want `this inner loop accumulates into an output row invariant in outer variable i — all of it is loaded and stored once per outer step; the remedy depends on whether the input is contiguous in d \(strip-mine the gather, outer-unroll the rank-1 update, register-tile a band\) — see PS1007 docs`
		}
	}
}

// The written row shares a name with a read operand: the unrolled pair
// would read it across partial writes, so only the advisory is offered.
func axpySelf(out, in []float64, n, dim int) {
	for i := 0; i < n; i++ {
		for d := 0; d < dim; d++ {
			out[d] += out[i] * in[i*dim+d] // want `this inner loop accumulates into an output row invariant in outer variable i — all of it is loaded and stored once per outer step; the remedy depends on whether the input is contiguous in d \(strip-mine the gather, outer-unroll the rank-1 update, register-tile a band\) — see PS1007 docs`
		}
	}
}
