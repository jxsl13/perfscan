package ps6099operationreach

import (
	"math"

	"ps6099operationreach/simdops"
)

func ApplyDeadF64(dst []float64) {
	if false {
		_ = math.Exp(dst[0])
	}
	for lane := 0; lane < len(dst); lane += 2 {
		dst[lane] = dst[lane]
		dst[lane+1] = dst[lane+1]
	}
}

func ApplyUnrelatedF64(dst []float64) {
	_ = math.Sin(1)
	for lane := 0; lane < len(dst); lane += 2 {
		dst[lane] = dst[lane]
		dst[lane+1] = dst[lane+1]
	}
}

func ApplyPartialF64(dst []float64) {
	for lane := 0; lane < len(dst); lane += 2 {
		dst[lane] = math.Cos(dst[lane])
		dst[lane+1] = dst[lane+1]
	}
}

func ApplyF64(dst []float64) {
	for lane := 0; lane < len(dst); lane += 2 {
		dst[lane] = math.Log(dst[lane])
		dst[lane+1] = math.Log(dst[lane+1])
	}
}

func ApplyVectorF64(dst []float64) {
	simdops.CbrtF64(dst)
}

func noDeadOperationEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func noUnrelatedOperationEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func noPartialLaneOperationEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cos(input[index])
	}
}

func linkedLaneOperationEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log exactly once per independent output element.*ApplyF64.*multi-lane vector-width loop`
		output[index] = math.Log(input[index])
	}
}

func linkedVectorCallEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cbrt exactly once per independent output element.*ApplyVectorF64.*SIMD/vector-backed via simdops.CbrtF64`
		output[index] = math.Cbrt(input[index])
	}
}
