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

func ApplyAtanF64(dst []float64) {
	for range [1]struct{}{} {
		simdops.AtanF64(dst)
	}
}

func exactOneRangeEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Atan exactly once per independent output element.*ApplyAtanF64`
		output[index] = math.Atan(input[index])
	}
}

func ApplyAsinF64(dst []float64) {
	switch value := any(1).(type) {
	case int:
		_ = value
		simdops.AsinF64(dst)
	}
}

func concreteTypeSwitchEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Asin exactly once per independent output element.*ApplyAsinF64`
		output[index] = math.Asin(input[index])
	}
}

func ApplyGammaF64(dst []float64) {
	func(values []float64) { simdops.GammaF64(values) }(dst)
}

func iifeParameterEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Gamma exactly once per independent output element.*ApplyGammaF64`
		output[index] = math.Gamma(input[index])
	}
}

func ApplyErfcF64(dst []float64) {
	values := dst
	func() { simdops.ErfcF64(values) }()
}

func iifeCapturedAliasEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Erfc exactly once per independent output element.*ApplyErfcF64`
		output[index] = math.Erfc(input[index])
	}
}

func ApplyExp2F64(dst []float64) {
	func(outer []float64) {
		values := outer
		func(inner []float64) {
			func() { simdops.Exp2F64(inner) }()
		}(values)
	}(dst)
}

func nestedIIFESequenceEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp2 exactly once per independent output element.*ApplyExp2F64`
		output[index] = math.Exp2(input[index])
	}
}

func ApplyAcosF64(dst []float64) {
	switch any(1).(type) {
	case string:
	default:
		simdops.AcosF64(dst)
	}
}

func concreteTypeSwitchDefaultEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Acos exactly once per independent output element.*ApplyAcosF64`
		output[index] = math.Acos(input[index])
	}
}

func ApplySinhF64(dst []float64) {
	switch any(1).(type) {
	case interface{}:
		simdops.SinhF64(dst)
	}
}

func concreteTypeSwitchInterfaceEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Sinh exactly once per independent output element.*ApplySinhF64`
		output[index] = math.Sinh(input[index])
	}
}

func ZeroRangeExpm1F64(dst []float64) {
	for range [0]struct{}{} {
		simdops.Expm1F64(dst)
	}
}

func noZeroRangeEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Expm1(input[index])
	}
}

func MultiRangeLog1pF64(dst []float64) {
	for range [2]struct{}{} {
		simdops.Log1pF64(dst)
	}
}

func noMultiRangeEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log1p(input[index])
	}
}

func UnknownRangeLog2F64(dst, values []float64) {
	for range values {
		simdops.Log2F64(dst)
	}
}

func noUnknownRangeEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Log2(input[index])
	}
}

func ConditionalRangeSinF64(dst []float64, enabled bool) {
	for range [1]struct{}{} {
		if enabled {
			simdops.SinF64(dst)
		}
	}
}

func noConditionalRangeEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func EscapingRangeCosF64(dst []float64, stop bool) {
	for range [1]struct{}{} {
		if stop {
			break
		}
		simdops.CosF64(dst)
	}
}

func noEscapingRangeEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cos(input[index])
	}
}

func UnknownTypeSwitchCoshF64(dst []float64, value any) {
	switch value.(type) {
	case int:
		simdops.CoshF64(dst)
	}
}

func noUnknownTypeSwitchEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Cosh(input[index])
	}
}

type leafCode int

func NamedMismatchTanhF64(dst []float64) {
	switch any(leafCode(1)).(type) {
	case int:
		simdops.TanhF64(dst)
	}
}

func noNamedMismatchTypeSwitchEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Tanh(input[index])
	}
}

func NilTypeSwitchExpF64(dst []float64) {
	switch any(nil).(type) {
	case int:
		simdops.ExpF64(dst)
	}
}

func noNilTypeSwitchEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func ConditionalAliasErfF64(dst []float64, enabled bool) {
	var values []float64
	if enabled {
		values = dst
	}
	func() { simdops.ErfF64(values) }()
}

func InvokedClosureAliasErfF64(dst []float64) {
	values := dst
	mutate := func() { values = make([]float64, len(dst)) }
	mutate()
	simdops.ErfF64(values)
}

func rebindLeafAlias(values *[]float64) {
	*values = make([]float64, 1)
}

func AddressEscapedAliasErfF64(dst []float64) {
	values := dst
	rebindLeafAlias(&values)
	simdops.ErfF64(values)
}

func noConditionalAliasEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Erf(input[index])
	}
}

func ReboundAliasPowF64(dst []float64) {
	values := dst
	values = make([]float64, len(dst))
	func() { simdops.PowF64(values) }()
}

func noReboundAliasEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Pow(input[index], 9)
	}
}
