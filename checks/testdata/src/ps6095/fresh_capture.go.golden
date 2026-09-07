package ps6095

// Negative: the closure can rebind output to the input alias between dynamic
// entries of the inner loop. Capturing the fresh slice header invalidates the
// destination-freshness proof even though the mutation is in another CFG.
func freshOutputClosureRebind(weights []float64, token int, denominator float64) {
	output := make([]float64, 16)
	alias := weights
	rebind := func() {
		output = alias
	}
	for range 2 {
		for index := range output {
			output[index] = weights[token] / denominator
		}
		rebind()
	}
}

// Negative: the nested closure escapes its creator and retains the fresh
// output slice. Capture is transitive across function-literal contexts.
func freshOutputReturnedNested(weights []float64, token int, denominator float64) func() []float64 {
	output := make([]float64, 16)
	makeGetter := func() func() []float64 {
		return func() []float64 {
			return output
		}
	}
	for index := range output {
		output[index] = weights[token] / denominator
	}
	return makeGetter()
}

// Negative: a read-only closure still exposes output's backing array. After
// weights receives that slice, stores through output may mutate the quotient
// input in the same iteration.
func freshOutputReadOnlyGetter(weights []float64, token int, denominator float64) {
	output := make([]float64, 16)
	getOutput := func() []float64 {
		return output
	}
	weights = getOutput()
	for index := range output {
		output[index] = weights[token] / denominator
	}
}

// Negative regression: direct rebinding already invalidates freshness.
func freshOutputDirectRebind(weights []float64, token int, denominator float64) {
	output := make([]float64, 16)
	output = weights
	for index := range output {
		output[index] = weights[token] / denominator
	}
}

func rebindFreshOutput(output *[]float64, weights []float64) {
	*output = weights
}

// Negative regression: exposing the slice header by address to an opaque call
// already invalidates freshness.
func freshOutputOpaqueRebind(weights []float64, token int, denominator float64) {
	output := make([]float64, 16)
	rebindFreshOutput(&output, weights)
	for index := range output {
		output[index] = weights[token] / denominator
	}
}

// Negative: returning output before the outer loop can re-enter exposes its
// backing array while the inner loop remains dynamically live.
func freshOutputReturnExposure(weights []float64, token int, denominator float64) []float64 {
	output := make([]float64, 16)
	for range 2 {
		for index := range output {
			output[index] = weights[token] / denominator
		}
		if token < 0 {
			return output
		}
	}
	return nil
}

// Negative: sending output can publish an alias before the next inner-loop
// entry.
func freshOutputSendExposure(weights []float64, token int, denominator float64, published chan<- []float64) {
	output := make([]float64, 16)
	for range 2 {
		for index := range output {
			output[index] = weights[token] / denominator
		}
		published <- output
	}
}

// Negative: storing output in an interface exposes its slice header before a
// later dynamic entry.
func freshOutputInterfaceExposure(weights []float64, token int, denominator float64) any {
	output := make([]float64, 16)
	var exposed any
	for range 2 {
		for index := range output {
			output[index] = weights[token] / denominator
		}
		exposed = output
	}
	return exposed
}

func consumeFreshOutput([]float64) {}

// Negative: an opaque call may retain the fresh destination and create an
// alias before the next inner-loop entry.
func freshOutputOpaqueExposure(weights []float64, token int, denominator float64) {
	output := make([]float64, 16)
	for range 2 {
		for index := range output {
			output[index] = weights[token] / denominator
		}
		consumeFreshOutput(output)
	}
}
