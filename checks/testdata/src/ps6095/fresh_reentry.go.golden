package ps6095

// Negative: output is fresh on the outer loop's first entry, but aliases
// weights when the inner loop is entered again. An assignment after the inner
// loop is therefore not safely "after" all dynamic executions of that loop.
func freshOutputAcrossOuterReentry(weights []float64, token int, denominator float64) {
	output := make([]float64, 16)
	for range 2 {
		for index := range output {
			output[index] = weights[token] / denominator
		}
		output = weights
	}
}

// Negative: the same alias can reach a later dynamic entry through a backward
// goto even though it is lexically after the analyzed loop.
func freshOutputAcrossGoto(weights []float64, token int, denominator float64) {
	output := make([]float64, 16)
again:
	for index := range output {
		output[index] = weights[token] / denominator
	}
	output = weights
	if token > 0 {
		token--
		goto again
	}
}
