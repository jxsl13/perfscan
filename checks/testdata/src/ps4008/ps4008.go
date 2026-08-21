package ps4008

func matmul(a, b, c [][]float64) {
	for i := range a {
		for j := range b[0] {
			sum := 0.0
			for k := range b { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				sum += a[i][k] * b[k][j]
			}
			c[i][j] = sum
		}
	}
}

// Counted middle loop is not the canonical range-over-b[0] shape: reported,
// but no automatic fix.
func matmulCounted(a, b, c [][]float64, n int) {
	for i := range a {
		for j := 0; j < n; j++ {
			sum := 0.0
			for k := range b { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				sum += a[i][k] * b[k][j]
			}
			c[i][j] = sum
		}
	}
}

// In-place matmul (output is an input): reported, but the axpy rewrite would
// change which overwritten elements later iterations read — no automatic fix.
func matmulInPlace(a, b [][]float64) {
	for i := range a {
		for j := range b[0] {
			sum := 0.0
			for k := range b { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				sum += a[i][k] * b[k][j]
			}
			a[i][j] = sum
		}
	}
}

// axpy form: accumulates into indexed destination, not a scalar — silent.
func matmulAxpy(a, b, c [][]float64) {
	for i := range a {
		for k := range b {
			aik := a[i][k]
			for j := range b[0] {
				c[i][j] += aik * b[k][j]
			}
		}
	}
}

// Depth 2 is a plain dot product, not a matmul nest: silent.
func dot(a, b []float64) float64 {
	s := 0.0
	for j := 0; j < 4; j++ {
		for k := range a {
			s += a[k] * b[k]
		}
	}
	return s
}
