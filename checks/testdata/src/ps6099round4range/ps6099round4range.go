package ps6099round4range

import (
	"math"

	"ps6099round4range/simdops"
)

func PointerArrayExpF64(dst []float64) {
	for range &[1]struct{}{} {
		simdops.ExpF64(dst)
	}
}

func pointerArrayEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*PointerArrayExpF64`
		output[index] = math.Exp(input[index])
	}
}

func PointerZeroLogF64(dst []float64) {
	for range &[0]struct{}{} {
		simdops.LogF64(dst)
	}
}

func noPointerZeroEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log(input[index])
	}
}

func PointerMultiSinF64(dst []float64) {
	for range &[2]struct{}{} {
		simdops.SinF64(dst)
	}
}

func noPointerMultiEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func ContinueAfterLog2F64(dst []float64) {
	for range [1]struct{}{} {
		simdops.Log2F64(dst)
		continue
	}
}

func continueAfterEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log2 exactly once per independent output element.*ContinueAfterLog2F64`
		output[index] = math.Log2(input[index])
	}
}

func ConditionalContinueBeforeTanF64(dst []float64, stop bool) {
	for range [1]struct{}{} {
		if stop {
			continue
		}
		simdops.TanF64(dst)
	}
}

func noConditionalContinueEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Tan(input[index])
	}
}

func LabeledContinueAfterTanhF64(dst []float64) {
outer:
	for range &[1]struct{}{} {
		simdops.TanhF64(dst)
		continue outer
	}
}

func labeledContinueAfterEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Tanh exactly once per independent output element.*LabeledContinueAfterTanhF64`
		output[index] = math.Tanh(input[index])
	}
}
