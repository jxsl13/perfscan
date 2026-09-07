package ps6095

type ps6095Float interface {
	~float32 | ~float64
}

type ps6095Mixed interface {
	~int | ~float64
}

func genericQuotient[T ps6095Float](output, gradient []T, weight, denominator T) {
	for index := range output {
		output[index] += gradient[index] * (weight / denominator) // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

func exactGenericQuotient[T ps6095Float](numerator, denominator T) T {
	return numerator / denominator
}

func genericHelperQuotient[T ps6095Float](output, gradient []T, weight, denominator T) {
	for index := range output {
		output[index] += gradient[index] * exactGenericQuotient[T](weight, denominator) // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

// Negative: the constraint includes integer division, so floating-point
// hoisting semantics have not been proven for every possible instantiation.
func mixedQuotient[T ps6095Mixed](output []T, numerator, denominator T) {
	for index := range output {
		output[index] = numerator / denominator
	}
}
