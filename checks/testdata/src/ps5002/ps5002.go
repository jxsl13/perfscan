package ps5002

func gram(m [][]float64, x []float64) {
	for i := range x {
		for j := range x {
			m[i][j] += x[i] * x[j] // want `m\[i\]\[j\] accumulates a symmetric product over full ranges of i and j — every off-diagonal element is computed twice; accumulate one triangle and mirror it once \(bit-identical\)`
		}
	}
}

// Already triangular: silent.
func triangle(m [][]float64, x []float64) {
	for i := range x {
		for j := 0; j <= i; j++ {
			m[i][j] += x[i] * x[j]
		}
	}
}

// Different vectors are not symmetric: silent.
func outer(m [][]float64, x, y []float64) {
	for i := range x {
		for j := range y {
			m[i][j] += x[i] * y[j]
		}
	}
}
