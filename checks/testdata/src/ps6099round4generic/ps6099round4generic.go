package ps6099round4generic

import (
	"math"

	"ps6099round4generic/simdops"
)

func GenericSliceExpF64[T ~[]float64](dst T) {
	simdops.ExpF64(dst)
}

func directGenericEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*GenericSliceExpF64`
		output[index] = math.Exp(input[index])
	}
}

func GenericSliceIIFEGammaF64[T ~[]float64](dst T) {
	func(values T) {
		simdops.GammaF64([]float64(values))
	}(dst)
}

func iifeGenericEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Gamma exactly once per independent output element.*GenericSliceIIFEGammaF64`
		output[index] = math.Gamma(input[index])
	}
}
