package ps6099ignoredunknown

import "math"

func noUnknownCalledSignatureEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}
