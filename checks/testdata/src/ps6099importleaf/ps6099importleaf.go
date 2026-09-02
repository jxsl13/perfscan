package ps6099importleaf

import (
	"math"

	simd "ps6099importleaf/simdops"
)

func ApplyExpF64(dst []float64) {
	simd.ExpF64(dst)
}

func transform(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ApplyExpF64.*SIMD/vector-backed via simd.ExpF64`
		output[index] = math.Exp(input[index])
	}
}

func ApplyLogF64(dst []float64) {
	if simd.Available() {
		for index := range dst {
			dst[index] = math.Log(dst[index])
		}
	}
}

func noFeatureCheckEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log(input[index])
	}
}

func ApplySinF64(dst []float64) {
	for index := range dst {
		dst[index] = simd.Sin(dst[index])
	}
}

func noScalarImportedEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func ApplyCosF64(dst []float64) {
	simd.ExpF64(dst)
}

func noMismatchedOperationEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cos(input[index])
	}
}
