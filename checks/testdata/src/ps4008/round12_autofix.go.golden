package ps4008

// Ragged bounds and row overlap are runtime properties for [][]float64, so
// all three exact counterexamples remain advisory diagnostics.
func sourcePanicStoreOrderMustStayAdvisoryRound12(a, b, c [][]float64) {
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

func outputPanicStoreOrderMustStayAdvisoryRound12(a, b, c [][]float64) {
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

func sharedRowsMustStayAdvisoryRound12(a, b, c [][]float64) {
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
