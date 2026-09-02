package ps6099interfaceswitch

import (
	"math"

	"ps6099interfaceswitch/simdops"
)

func deadDynamicTypeMismatch(dst []float64) {
	switch any(1) {
	case int64(1), int64(2):
		simdops.SinhF64(dst)
	}
}

func liveDynamicTypeMatch(dst []float64) {
	switch any(int64(1)) {
	case int(1), int64(1):
		simdops.TanhF64(dst)
	}
}

func deadDynamicValueMismatch(dst []float64) {
	switch any(false) {
	case true:
		simdops.ExpF64(dst)
	}
}

func liveDynamicValueMatch(dst []float64) {
	switch any(false) {
	case false:
		simdops.LogF64(dst)
	}
}

func liveDefault(dst []float64) {
	switch any(1) {
	case int64(1):
		return
	default:
		simdops.CosF64(dst)
	}
}

func deadDefaultBeforeSelectedCase(dst []float64) {
	switch any(1) {
	default:
		simdops.SinF64(dst)
	case int(1):
		return
	}
}

func liveFallthrough(dst []float64) {
	switch any(int64(1)) {
	case int64(1):
		fallthrough
	case int(1):
		simdops.AcosF64(dst)
	}
}

func deadFallthrough(dst []float64) {
	switch any(1) {
	case int64(1):
		fallthrough
	case int64(2):
		simdops.AsinF64(dst)
	}
}

func conservativelyReachableUnknown(dst []float64, tag any) {
	switch tag {
	case int64(1):
		simdops.AtanF64(dst)
	}
}

type interfaceCode int

func liveNamedDynamicType(dst []float64) {
	switch any(interfaceCode(1)) {
	case interfaceCode(1):
		simdops.CbrtF64(dst)
	}
}

func noDeadDynamicTypeMismatchEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sinh(input[index])
	}
}

func liveDynamicTypeMatchEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Tanh exactly once per independent output element.*liveDynamicTypeMatch`
		output[index] = math.Tanh(input[index])
	}
}

func noDeadDynamicValueMismatchEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func liveDynamicValueMatchEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log exactly once per independent output element.*liveDynamicValueMatch`
		output[index] = math.Log(input[index])
	}
}

func liveDefaultEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cos exactly once per independent output element.*liveDefault`
		output[index] = math.Cos(input[index])
	}
}

func noDeadDefaultEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func liveFallthroughEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Acos exactly once per independent output element.*liveFallthrough`
		output[index] = math.Acos(input[index])
	}
}

func noDeadFallthroughEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Asin(input[index])
	}
}

func conservativelyReachableUnknownEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Atan exactly once per independent output element.*conservativelyReachableUnknown`
		output[index] = math.Atan(input[index])
	}
}

func liveNamedDynamicTypeEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cbrt exactly once per independent output element.*liveNamedDynamicType`
		output[index] = math.Cbrt(input[index])
	}
}
