package ps6099round5swap

import (
	"math"
	"ps6099round5swap/simdops"
)

func SwappedExpF64(dst []float64) {
	scratch := make([]float64, len(dst))
	scratch, dst = dst, scratch
	simdops.ExpF64(dst)
}
func PreservedLogF64(dst []float64) {
	scratch := make([]float64, len(dst))
	dst, scratch = scratch, dst
	simdops.LogF64(scratch)
}
func OverwrittenSinF64(dst []float64) {
	scratch := make([]float64, len(dst))
	dst, dst = dst, scratch
	simdops.SinF64(dst)
}
func SelectorCosF64(dst []float64) {
	var state struct{ values []float64 }
	state.values = make([]float64, len(dst))
	state.values, dst = dst, state.values
	simdops.CosF64(dst)
}
func TupleIIFETanF64(dst []float64) {
	func(values []float64) {
		scratch := make([]float64, len(values))
		scratch, values = values, scratch
		simdops.TanF64(values)
	}(dst)
}
func ParallelAliasCbrtF64(dst []float64) {
	scratch := make([]float64, len(dst))
	first, second := dst, scratch
	first, second = second, first
	simdops.CbrtF64(second)
}

func scalarExp(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func scalarLog(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log exactly once per independent output element`
		output[index] = math.Log(input[index])
	}
}

func scalarSin(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func scalarCos(output, input []float64) {
	for index := range input {
		output[index] = math.Cos(input[index])
	}
}

func scalarTan(output, input []float64) {
	for index := range input {
		output[index] = math.Tan(input[index])
	}
}

func scalarCbrt(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cbrt exactly once per independent output element`
		output[index] = math.Cbrt(input[index])
	}
}
