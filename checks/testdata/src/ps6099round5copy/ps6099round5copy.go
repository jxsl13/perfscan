package ps6099round5copy

import (
	"math"
	"ps6099round5copy/simdops"
)

func CopyExpF64(dst []float64) { simdops.ExpF64([2]float64(dst)) }
func CopyThenSliceLogF64(dst []float64) {
	values := [2]float64(dst)
	simdops.LogF64(values[:])
}
func CopyIIFESinF64(dst []float64) {
	func(values [2]float64) { simdops.SinF64(values[:]) }([2]float64(dst))
}
func PointerCosF64(dst []float64) { simdops.CosF64((*[2]float64)(dst)) }

type band []float64

func NamedSliceTanF64(dst []float64) { simdops.TanF64(band(dst)) }

type arrayBand [2]float64

func (arrayBand) SinhSIMDF64()             {}
func (*arrayBand) CoshSIMDF64()            {}
func ValueReceiverSinhF64(dst []float64)   { arrayBand(dst).SinhSIMDF64() }
func PointerReceiverCoshF64(dst []float64) { (*arrayBand)(dst).CoshSIMDF64() }
func GenericCopyGammaF64(dst []float64)    { simdops.GammaF64([2]float64(dst)) }

func scalarExp(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func scalarLog(output, input []float64) {
	for index := range input {
		output[index] = math.Log(input[index])
	}
}

func scalarSin(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func scalarCos(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cos exactly once per independent output element`
		output[index] = math.Cos(input[index])
	}
}

func scalarTan(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Tan exactly once per independent output element`
		output[index] = math.Tan(input[index])
	}
}

func scalarSinh(output, input []float64) {
	for index := range input {
		output[index] = math.Sinh(input[index])
	}
}

func scalarCosh(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cosh exactly once per independent output element`
		output[index] = math.Cosh(input[index])
	}
}

func scalarGamma(output, input []float64) {
	for index := range input {
		output[index] = math.Gamma(input[index])
	}
}
