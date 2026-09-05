package ps6099adversarial

import "math"

func ExpSIMDF64(dst []float64)

type scalar64 float64
type namedBand []scalar64

func LogSIMDF64(dst namedBand)

type aliasScalar = float64
type aliasBand []aliasScalar

func Log1pSIMDF64(dst aliasBand)

type dynamicBackend interface {
	SinSIMDF64([]float64)
}

func ApplySinF64(dst []float64, implementation dynamicBackend) {
	implementation.SinSIMDF64(dst)
}

type cosineBackend struct{}

func (cosineBackend) CosSIMDF64([]float64) {}

func ApplyCosF64(dst []float64) {
	cosineBackend{}.CosSIMDF64(dst)
}

type tangentBackend struct{}

func (tangentBackend) TanSIMDF64([]float64) {}

func ApplyTanF64(dst []float64) {
	tangentBackend.TanSIMDF64(tangentBackend{}, dst)
}

type sinhBand []float64

func (sinhBand) SinhSIMDF64() {}

func ApplySinhF64(dst []float64) {
	sinhBand(dst).SinhSIMDF64()
}

type genericBackend[T ~float64] struct{}

func (genericBackend[T]) CoshSIMD(dst []T) {}

func ApplyCosh[T ~float64](dst []T) {
	genericBackend[T]{}.CoshSIMD(dst)
}

func expWithFlag(_ bool, value float64) float64 {
	return math.Exp(value)
}

func expOfLog(value float64) float64 {
	return math.Exp(math.Log(value))
}

func square(value float64) float64 {
	return value * value
}

func expOfSquare(value float64) float64 {
	return math.Exp(square(value))
}

func expWithDeadLog(value float64) float64 {
	return expWithFlag(false && math.Log(value) > 0, value)
}

func expWithLiveLog(value float64) float64 {
	return expWithFlag(true && math.Log(value) > 0, value)
}

func expWithMaybeLog(condition bool, value float64) float64 {
	return expWithFlag(condition && math.Log(value) > 0, value)
}

func alwaysPanic(value float64) float64 {
	panic(value)
}

func expAfterPanic(value float64) float64 {
	return math.Exp(alwaysPanic(value))
}

func neverReturns(value float64) float64 {
	for {
		_ = value
	}
}

func expAfterNonreturn(value float64) float64 {
	return math.Exp(neverReturns(value))
}

func expWithCopy(value float64, scratch []byte) float64 {
	return math.Exp(value + float64(copy(scratch, scratch)))
}

func zeroWithCopy(scratch []byte) float64 {
	return float64(copy(scratch, scratch))
}

func expViaNestedCopy(value float64, scratch []byte) float64 {
	return math.Exp(value + zeroWithCopy(scratch))
}

func expWithLength(value float64, scratch []byte) float64 {
	return math.Exp(value + float64(len(scratch)))
}

func noInterfaceDestination(output []any, input []float64) {
	for index := range input {
		output[index] = math.Exp(input[index])
	}
}

func validFloatDestination(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = math.Exp(input[index])
	}
}

func noIncompatibleNamedLeaf(output, input []float64) {
	for index := range input {
		output[index] = math.Log(input[index])
	}
}

func validAliasBand(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Log1p exactly once per independent output element.*Log1pSIMDF64`
		output[index] = math.Log1p(input[index])
	}
}

func noNestedScalarTranscendentals(output, input []float64) {
	for index := range input {
		output[index] = expOfLog(input[index])
	}
}

func zeroScalarLocalHelper(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = expOfSquare(input[index])
	}
}

func deadShortCircuitHelper(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = expWithDeadLog(input[index])
	}
}

func noLiveNestedHelper(output, input []float64) {
	for index := range input {
		output[index] = expWithLiveLog(input[index])
	}
}

func noMaybeNestedHelper(output, input []float64, condition bool) {
	for index := range input {
		output[index] = expWithMaybeLog(condition, input[index])
	}
}

func noScalarAfterPanickingArgument(output, input []float64) {
	for index := range input {
		output[index] = expAfterPanic(input[index])
	}
}

func noScalarAfterNonreturningArgument(output, input []float64) {
	for index := range input {
		output[index] = expAfterNonreturn(input[index])
	}
}

func noRuntimeCopyHiddenByScalarHelper(output, input []float64, scratch []byte) {
	for index := range input {
		output[index] = expWithCopy(input[index], scratch)
	}
}

func noNestedRuntimeCopyHiddenByScalarHelper(output, input []float64, scratch []byte) {
	for index := range input {
		output[index] = expViaNestedCopy(input[index], scratch)
	}
}

func pureBuiltinInScalarHelper(output, input []float64, scratch []byte) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = expWithLength(input[index], scratch)
	}
}

func deadLoopScalarCall(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		if false {
			_ = math.Log(input[index])
		}
		output[index] = math.Exp(input[index])
	}
}

func noInterfaceDispatchEvidence(output, input []float64) {
	for index := range input {
		output[index] = math.Sin(input[index])
	}
}

func concreteMethodEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cos exactly once per independent output element.*ApplyCosF64`
		output[index] = math.Cos(input[index])
	}
}

func concreteMethodExpressionEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Tan exactly once per independent output element.*ApplyTanF64`
		output[index] = math.Tan(input[index])
	}
}

func concreteSequenceReceiverEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Sinh exactly once per independent output element.*ApplySinhF64`
		output[index] = math.Sinh(input[index])
	}
}

func concreteGenericMethodEvidence(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Cosh exactly once per independent output element.*ApplyCosh`
		output[index] = math.Cosh(input[index])
	}
}

func round2ExpSecond(_ float64, value float64) float64 {
	return math.Exp(value)
}

func round2IgnoredPanickingArgument(output, input, scratch []float64) {
	for index := range input {
		output[index] = round2ExpSecond(scratch[1], input[index])
	}
}

func round2ExpWithNestedIgnoredArgument(value float64, scratch []float64) float64 {
	return round2ExpSecond(scratch[1], value)
}

func round2NestedIgnoredPanickingArgument(output, input, scratch []float64) {
	for index := range input {
		output[index] = round2ExpWithNestedIgnoredArgument(input[index], scratch)
	}
}

var round2MutationCount int

func round2MutateThenReturn(value float64) float64 {
	round2MutationCount++
	return value
}

func round2ExpAfterMutation(value float64) float64 {
	return math.Exp(round2MutateThenReturn(value))
}

func round2NestedMutation(output, input []float64) {
	for index := range input {
		output[index] = round2ExpAfterMutation(input[index])
	}
}

func round2GenericMutateThenReturn[T ~float64](value T) T {
	round2MutationCount++
	return value
}

func round2GenericExpAfterMutation[T ~float64](value T) float64 {
	return math.Exp(float64(round2GenericMutateThenReturn(value)))
}

func round2NestedGenericMutation(output, input []float64) {
	for index := range input {
		output[index] = round2GenericExpAfterMutation(input[index])
	}
}

func round2GenericIdentity[T ~float64](value T) T {
	return value
}

func round2GenericExp[T ~float64](value T) float64 {
	return math.Exp(float64(round2GenericIdentity[T](value)))
}

func round2ExplicitGenericChain(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = round2GenericExp[float64](input[index])
	}
}

func round2AllocateThenReturn(value float64) float64 {
	scratch := []byte{byte(value)}
	return value + float64(len(scratch)-1)
}

func round2ExpAfterAllocation(value float64) float64 {
	return math.Exp(round2AllocateThenReturn(value))
}

func round2NestedAllocation(output, input []float64) {
	for index := range input {
		output[index] = round2ExpAfterAllocation(input[index])
	}
}

func round2MaybeReturn(value float64, returnNow bool) float64 {
	if returnNow {
		return value
	}
	for {
	}
}

func round2ExpAfterMaybeReturn(value float64, returnNow bool) float64 {
	return math.Exp(round2MaybeReturn(value, returnNow))
}

func round2NotAllPathsReturn(output, input []float64, returnNow bool) {
	for index := range input {
		output[index] = round2ExpAfterMaybeReturn(input[index], returnNow)
	}
}

func round2ExpOfCheapPow(value float64) float64 {
	return math.Exp(math.Pow(value, 8))
}

func round2CheapPowInsideExp(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = round2ExpOfCheapPow(input[index])
	}
}

func round2PureBuiltinsDirect(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = math.Exp(input[index] + float64(len(input)-len(input)))
	}
}

func round2DeadShortCircuitScalar(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		if false && math.Log(input[index]) > 0 {
			return
		}
		output[index] = math.Exp(input[index])
	}
}

func round2DeadShortCircuitScalarNoExit(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		if false && math.Log(input[index]) > 0 {
			_ = input[index]
		}
		output[index] = math.Exp(input[index])
	}
}

func round2MutateBool() bool {
	round2MutationCount++
	return true
}

func round2ValueWithDeadMutation(value float64) float64 {
	if false && round2MutateBool() {
		return 0
	}
	return value
}

func round2ExpAfterDeadMutation(value float64) float64 {
	return math.Exp(round2ValueWithDeadMutation(value))
}

func round2NestedDeadMutation(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = round2ExpAfterDeadMutation(input[index])
	}
}
