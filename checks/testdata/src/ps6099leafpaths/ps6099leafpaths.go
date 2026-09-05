package ps6099leafpaths

import (
	"math"

	"ps6099leafpaths/simdops"
)

func DispatchedTanF64(dst []float64, avx bool) {
	if avx {
		simdops.TanF64(dst)
	} else {
		simdops.TanF64(dst)
	}
}

func allPathDispatchEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Tan exactly once per independent output element.*DispatchedTanF64`
		output[index] = math.Tan(input[index])
	}
}

func ImmediateDispatchedLogF64(dst []float64, avx bool) {
	func() {
		if avx {
			simdops.LogF64(dst)
		} else {
			simdops.LogF64(dst)
		}
	}()
}

func immediateAllPathDispatchEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log exactly once per independent output element.*ImmediateDispatchedLogF64`
		output[index] = math.Log(input[index])
	}
}

func ConditionalImmediateCbrtF64(dst []float64, enabled bool) {
	func() {
		if enabled {
			simdops.CbrtF64(dst)
		}
	}()
}

func noConditionalImmediateDispatchEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cbrt(input[index])
	}
}
