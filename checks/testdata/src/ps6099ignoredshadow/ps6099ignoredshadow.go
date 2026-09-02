package ps6099ignoredshadow

import (
	"math"
	_ "ps6099ignoredshadow/simdops"
)

func noShadowedQualifierEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func importedAfterDisjointShadowEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log exactly once per independent output element.*ApplyLogF64`
		output[index] = math.Log(input[index])
	}
}

func importedBeforeLaterShadowEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log10 exactly once per independent output element.*ApplyLog10F64`
		output[index] = math.Log10(input[index])
	}
}

func noShadowedCallbackEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func noParameterShadowEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Tan(input[index])
	}
}

func noInitShadowEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sinh(input[index])
	}
}

func noRangeShadowEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cosh(input[index])
	}
}
