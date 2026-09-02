package ps6099badsequence

import "math"

func LogSIMD(dst []int)

func noIntegerSequenceEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log(input[index])
	}
}
