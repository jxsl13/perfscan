package ps6099mixed

import "math"

func ExpSIMDMixed(dst []float64, scratch []float32)

func noMixedPrecisionEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}
