package ps6099round4iife

import (
	"math"

	"ps6099round4iife/simdops"
)

func OneVariadicErfcF64(dst []float64) {
	func(values ...[]float64) {
		simdops.ErfcF64(values[0])
	}(dst)
}

func oneVariadicEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Erfc exactly once per independent output element.*OneVariadicErfcF64`
		output[index] = math.Erfc(input[index])
	}
}

func MultiVariadicGammaF64(dst []float64) {
	func(values ...[]float64) {
		simdops.GammaF64(values[0])
	}(dst, make([]float64, len(dst)))
}

func multiVariadicEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Gamma exactly once per independent output element.*MultiVariadicGammaF64`
		output[index] = math.Gamma(input[index])
	}
}

func UnrelatedVariadicErfF64(dst []float64) {
	func(values ...[]float64) {
		simdops.ErfF64(values[1])
	}(dst, make([]float64, len(dst)))
}

func noUnrelatedVariadicEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Erf(input[index])
	}
}

func ZeroVariadicLog1pF64(dst []float64) {
	func(values ...[]float64) {
		simdops.Log1pF64(values[0])
	}()
}

func noZeroVariadicEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log1p(input[index])
	}
}

func EllipsisCoshF64(dst []float64) {
	values := [][]float64{dst}
	func(parts ...[]float64) {
		simdops.CoshF64(parts[0])
	}(values...)
}

func noEllipsisEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cosh(input[index])
	}
}

func MutatedVariadicSinhF64(dst []float64) {
	func(values ...[]float64) {
		values[0] = make([]float64, len(dst))
		simdops.SinhF64(values[0])
	}(dst)
}

func noMutatedVariadicEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sinh(input[index])
	}
}

func mutateVariadic([][]float64)

func EscapedVariadicCbrtF64(dst []float64) {
	func(values ...[]float64) {
		mutateVariadic(values)
		simdops.CbrtF64(values[0])
	}(dst)
}

func noEscapedVariadicEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cbrt(input[index])
	}
}
