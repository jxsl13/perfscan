package ps6099boolswitch

import (
	"math"

	"ps6099boolswitch/simdops"
)

func deadTaggedBoolean(dst []float64) {
	switch false {
	case true:
		simdops.ExpF64(dst)
	}
}

func liveTaggedBoolean(dst []float64) {
	switch false {
	case false:
		simdops.LogF64(dst)
	}
}

func liveExpressionless(dst []float64) {
	switch {
	case true:
		simdops.SinF64(dst)
	}
}

func deadExpressionless(dst []float64) {
	switch {
	case false:
		simdops.CosF64(dst)
	}
}

func liveMultipleCases(dst []float64) {
	switch false {
	case true, false:
		simdops.AcosF64(dst)
	}
}

func deadMultipleCases(dst []float64) {
	switch false {
	case true, true:
		simdops.AsinF64(dst)
	}
}

func liveDefault(dst []float64) {
	switch false {
	case true:
		return
	default:
		simdops.AtanF64(dst)
	}
}

func deadDefaultBeforeSelectedCase(dst []float64) {
	switch false {
	default:
		simdops.CbrtF64(dst)
	case false:
		return
	}
}

func liveFallthrough(dst []float64) {
	switch false {
	case false:
		fallthrough
	case true:
		simdops.CoshF64(dst)
	}
}

func deadFallthrough(dst []float64) {
	switch false {
	case true:
		fallthrough
	case true:
		simdops.ErfF64(dst)
	}
}

func conservativelyReachableUnknownTag(dst []float64, tag bool) {
	switch tag {
	case false:
		simdops.ErfcF64(dst)
	}
}

func deadFalseAndUnknown(dst []float64, unknown bool) {
	if (false && unknown) || (unknown && false) {
		simdops.GammaF64(dst)
	}
}

func liveNegatedFalseAndUnknown(dst []float64, unknown bool) {
	if !(false && unknown) {
		simdops.TanhF64(dst)
	}
}

func liveTrueOrUnknown(dst []float64, unknown bool) {
	if (true || unknown) && (unknown || true) {
		simdops.Exp2F64(dst)
	}
}

func deadNegatedTrueOrUnknown(dst []float64, unknown bool) {
	if !(true || unknown) {
		simdops.Expm1F64(dst)
	}
}

func deadNestedLogicalCondition(dst []float64, unknown bool) {
	if (true || unknown) && !(!(false && unknown)) {
		simdops.Log1pF64(dst)
	}
}

func liveNestedLogicalCondition(dst []float64, unknown bool) {
	if !((false && unknown) || false) {
		simdops.Log2F64(dst)
	}
}

func conservativelyReachableUnknownLogicalCondition(dst []float64, unknown bool) {
	if unknown && true {
		simdops.TanF64(dst)
	}
}

func noDeadTaggedBooleanEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func liveTaggedBooleanEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log exactly once per independent output element.*liveTaggedBoolean`
		output[index] = math.Log(input[index])
	}
}

func liveExpressionlessEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Sin exactly once per independent output element.*liveExpressionless`
		output[index] = math.Sin(input[index])
	}
}

func noDeadExpressionlessEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cos(input[index])
	}
}

func liveMultipleCasesEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Acos exactly once per independent output element.*liveMultipleCases`
		output[index] = math.Acos(input[index])
	}
}

func noDeadMultipleCasesEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Asin(input[index])
	}
}

func liveDefaultEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Atan exactly once per independent output element.*liveDefault`
		output[index] = math.Atan(input[index])
	}
}

func noDeadDefaultEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cbrt(input[index])
	}
}

func liveFallthroughEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cosh exactly once per independent output element.*liveFallthrough`
		output[index] = math.Cosh(input[index])
	}
}

func noDeadFallthroughEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Erf(input[index])
	}
}

func conservativelyReachableUnknownTagEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Erfc exactly once per independent output element.*conservativelyReachableUnknownTag`
		output[index] = math.Erfc(input[index])
	}
}

func noDeadFalseAndUnknownEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Gamma(input[index])
	}
}

func liveNegatedFalseAndUnknownEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Tanh exactly once per independent output element.*liveNegatedFalseAndUnknown`
		output[index] = math.Tanh(input[index])
	}
}

func liveTrueOrUnknownEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp2 exactly once per independent output element.*liveTrueOrUnknown`
		output[index] = math.Exp2(input[index])
	}
}

func noDeadNegatedTrueOrUnknownEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Expm1(input[index])
	}
}

func noDeadNestedLogicalConditionEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log1p(input[index])
	}
}

func liveNestedLogicalConditionEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log2 exactly once per independent output element.*liveNestedLogicalCondition`
		output[index] = math.Log2(input[index])
	}
}

func conservativelyReachableUnknownLogicalConditionEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Tan exactly once per independent output element.*conservativelyReachableUnknownLogicalCondition`
		output[index] = math.Tan(input[index])
	}
}
