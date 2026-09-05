package ps6099round5iife

import (
	"math"
	"ps6099round5iife/simdops"
)

func EqualExpF64(dst []float64) { _ = func() bool { simdops.ExpF64(dst); return true }() == true }

func scalarExp(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element`
		output[index] = math.Exp(input[index])
	}
}
func LeftAndLogF64(dst []float64) { _ = func() bool { simdops.LogF64(dst); return true }() && true }

func scalarLog(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log exactly once per independent output element`
		output[index] = math.Log(input[index])
	}
}
func NestedUnarySinF64(dst []float64) {
	_ = !(func() bool { simdops.SinF64(dst); return true }() == true)
}

func scalarSin(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Sin exactly once per independent output element`
		output[index] = math.Sin(input[index])
	}
}
func RightAndCosF64(dst []float64) { _ = true && func() bool { simdops.CosF64(dst); return true }() }

func scalarCos(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cos exactly once per independent output element`
		output[index] = math.Cos(input[index])
	}
}
func DeadRightTanF64(dst []float64) { _ = false && func() bool { simdops.TanF64(dst); return true }() }

func scalarTan(output, input []float64) {
	for index := range input {
		output[index] = math.Tan(input[index])
	}
}
func UnaryCbrtF64(dst []float64) { _ = !(func() bool { simdops.CbrtF64(dst); return true }()) }

func scalarCbrt(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cbrt exactly once per independent output element`
		output[index] = math.Cbrt(input[index])
	}
}
func ConditionalCoshF64(dst []float64) {
	if func() bool { simdops.CoshF64(dst); return true }() == true {
	}
}

func scalarCosh(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cosh exactly once per independent output element`
		output[index] = math.Cosh(input[index])
	}
}
func UnknownRightSinhF64(dst []float64) {
	unknown := len(dst) > 0
	_ = unknown && func() bool { simdops.SinhF64(dst); return true }()
}

func scalarSinh(output, input []float64) {
	for index := range input {
		output[index] = math.Sinh(input[index])
	}
}
