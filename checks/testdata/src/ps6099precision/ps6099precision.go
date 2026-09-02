package ps6099precision

import "math"

// The name and actual sequence precision contradict each other.
func ExpSIMDF32(dst []float64)

func noContradictoryNameEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}
