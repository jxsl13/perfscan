package ps6095

func genericFunctionTypeParameter[psQuotient5_30 any](output []float64, numerator, denominator float64) {
	for index := range output {
		output[index] = [1]float64{numerator / denominator}[0] // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

type genericPair[first, second any] struct{}

func (genericPair[psQuotient13_19, second]) genericMethod(output []float64, numerator, denominator float64) {
	for index := range output {
		output[index] = numerator / denominator // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

func (*genericPair[psQuotient19_19, second]) genericPointerMethod(output []float64, numerator, denominator float64) {
	for index := range output {
		output[index] = numerator / denominator // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

func deterministicGenericFallback[psQuotient25_19, psQuotient25_19_2 any](output []float64, numerator, denominator float64) {
	for index := range output {
		output[index] = numerator / denominator // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

func unrelatedGenericName[element any](output []float64, numerator, denominator float64) {
	for index := range output {
		output[index] = numerator / denominator // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

func genericLiteralScope[psQuotient38_20 any](output []float64, numerator, denominator float64) func() {
	return func() {
		for index := range output {
			output[index] = numerator / denominator // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
		}
	}
}
