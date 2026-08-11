package ps5001

func normalize(xs []float64, temp float64) {
	for i := range xs {
		xs[i] = xs[i] / temp // want `divide by loop-invariant temp on every iteration; a reciprocal multiply is NOT bit-identical \(≤1 ulp on up to 2/3 of inputs\) and only pays on a memory-free path — MOST FINDINGS SHOULD BE DECLINED`
	}
}

// The divisor is a reduction accumulator (softmax denominator): excluded.
func softmax(xs []float64) {
	sum := 0.0
	for _, v := range xs {
		sum += v
	}
	for i := range xs {
		xs[i] = xs[i] / sum
	}
}

// Integer division has no reciprocal-multiply rewrite: silent.
func intDiv(xs []int, d int) {
	for i := range xs {
		xs[i] = xs[i] / d
	}
}

// Divisor reassigned in the loop: not invariant, silent.
func varying(xs []float64, d float64) {
	for i := range xs {
		xs[i] = xs[i] / d
		d += 1
	}
}

func outsideLoop(a, b float64) float64 {
	return a / b
}
