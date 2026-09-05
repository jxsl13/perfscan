package ps6099genericcalled

import (
	"math"

	"ps6099genericcalled/simdops"
)

func ApplyExp[T ~float64](dst []T) {
	var scratch [1]T
	simdops.ExpF64(dst, scratch[:])
}

func genericCalledSignatureEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ApplyExp`
		output[index] = math.Exp(input[index])
	}
}
