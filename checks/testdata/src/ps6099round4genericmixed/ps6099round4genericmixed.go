package ps6099round4genericmixed

import (
	"math"

	"ps6099round4genericmixed/simdops"
)

func MixedExpF64[T ~[]float32 | ~[]float64](dst T) {
	simdops.Exp(dst)
}

func noMixedGenericEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}
