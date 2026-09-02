package ps6099deadleaf

import (
	"math"

	"ps6099deadleaf/simdops"
)

func ExpF64(dst []float64) {
	if false {
		simdops.ExpF64(dst)
	}
}

func LogF64(dst []float64) {
	switch 0 {
	case 1:
		simdops.LogF64(dst)
	}
}

func SinF64(dst []float64) {
	for range [0]int{} {
		simdops.SinF64(dst)
	}
}

func CosF64(dst []float64) {
	for iteration := 0; iteration < 0; iteration++ {
		simdops.CosF64(dst)
	}
}

func TanSIMDF64(dst []float64) {
	TanSIMDF64 := func([]float64) {}
	TanSIMDF64(dst)
}

type scalarBackend struct{}

func (scalarBackend) SinhSIMDF64([]float64) {}

func SinhF64(dst []float64) {
	SinhSIMDF64 := scalarBackend{}.SinhSIMDF64
	SinhSIMDF64(dst)
}

func AcosF64(dst []float64) {
	return
	simdops.AcosF64(dst)
}

func AsinF64(dst []float64) {
	panic("unreachable vector leaf")
	simdops.AsinF64(dst)
}

func AtanF64(dst []float64) {
	goto vector
	return
vector:
	simdops.AtanF64(dst)
}

func CbrtF64(dst []float64) {
	for {
		break
	}
	simdops.CbrtF64(dst)
}

func CoshF64(dst []float64) {
	for range [1]int{} {
		continue
		simdops.CoshF64(dst)
	}
}

func ErfF64(dst []float64) {
	for range [1]int{} {
		break
		simdops.ErfF64(dst)
	}
}

func ErfcF64(dst []float64) {
	if true {
		return
	}
	simdops.ErfcF64(dst)
}

func GammaF64(dst []float64) {
	if false {
		return
	}
	simdops.GammaF64(dst)
}

func TanhF64(dst []float64) {
	switch 0 {
	case 0:
		return
	}
	simdops.TanhF64(dst)
}

func Exp2F64(dst []float64) {
	switch 0 {
	case 1:
		return
	}
	simdops.Exp2F64(dst)
}

func Expm1F64(dst []float64) {
	switch 0 {
	case 0:
		fallthrough
	case 1:
		simdops.Expm1F64(dst)
	}
}

func Log1pF64(dst []float64) {
	goto done
	simdops.Log1pF64(dst)
done:
}

func Log2F64(dst []float64) {
	panic := func(any) {}
	panic(nil)
	simdops.Log2F64(dst)
}

func noFalseIfEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func noDeadSwitchEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log(input[index])
	}
}

func noZeroRangeEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func noZeroForEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cos(input[index])
	}
}

func noShadowedCallbackEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Tan(input[index])
	}
}

func noShadowedMethodAliasEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sinh(input[index])
	}
}

func noAfterReturnEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Acos(input[index])
	}
}

func noAfterPanicEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Asin(input[index])
	}
}

func reachableGotoTargetEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Atan exactly once per independent output element.*AtanF64`
		output[index] = math.Atan(input[index])
	}
}

func reachableAfterBreakEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cbrt exactly once per independent output element.*CbrtF64`
		output[index] = math.Cbrt(input[index])
	}
}

func noAfterContinueEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cosh(input[index])
	}
}

func noAfterBreakEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Erf(input[index])
	}
}

func noAfterConstantIfReturnEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Erfc(input[index])
	}
}

func reachableAfterFalseIfEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Gamma exactly once per independent output element.*GammaF64`
		output[index] = math.Gamma(input[index])
	}
}

func noAfterSelectedSwitchReturnEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Tanh(input[index])
	}
}

func reachableAfterUnselectedSwitchReturnEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp2 exactly once per independent output element.*Exp2F64`
		output[index] = math.Exp2(input[index])
	}
}

func reachableSwitchFallthroughEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Expm1 exactly once per independent output element.*Expm1F64`
		output[index] = math.Expm1(input[index])
	}
}

func noGotoSkippedEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log1p(input[index])
	}
}

func reachableAfterShadowedPanicEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log2 exactly once per independent output element.*Log2F64`
		output[index] = math.Log2(input[index])
	}
}
