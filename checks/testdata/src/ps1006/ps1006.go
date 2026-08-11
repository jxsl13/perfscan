package ps1006

func columnSums(a []float64, rows, cols int, out []float64) {
	for c := 0; c < cols; c++ {
		s := 0.0
		for r := 0; r < rows; r++ {
			s += a[r*cols+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; interchange the loops so it is walked contiguously \(bit-identical per output element\), or gather into contiguous scratch when the scan cannot be interchanged`
		}
		out[c] = s
	}
}

// The inner variable is the additive (contiguous) part: already row-major.
func rowSums(a []float64, rows, cols int, out []float64) {
	for r := 0; r < rows; r++ {
		s := 0.0
		for c := 0; c < cols; c++ {
			s += a[r*cols+c]
		}
		out[r] = s
	}
}

// Not a reduction (writes per inner element): interchange changes nothing
// to accumulate, silent.
func scatter(a, b []float64, rows, cols int) {
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			b[r*cols+c] = a[r*cols+c] * 2
		}
	}
}
