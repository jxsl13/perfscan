package ps6099calleeprecision

import (
	"math"

	"ps6099calleeprecision/simdops"
)

func ApplyExpF64(dst []float64) {
	simdops.ExpF32(dst)
}

func noContradictoryCalleeEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}
