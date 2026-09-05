package ps6101round7

import (
	"math/rand"
	"testing"
)

var sink float64

func BenchmarkIncompatibleCommaOKRetainsSource(b *testing.B) {
	weight := rand.NormFloat64()
	var boxed any = weight
	_, ok := boxed.(float32)
	_ = ok
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkCompatibleCommaOKCarriesSource(b *testing.B) {
	weight := rand.NormFloat64()
	var boxed any = weight
	asserted, ok := boxed.(float64)
	_ = ok
	total := asserted
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkCommaOKSliceAliasOverwriteIsSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	var boxed any = weights
	asserted, ok := boxed.([]float64)
	_ = ok
	asserted[0] = 1
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkTypeSwitchGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	var boxed any = weights
	switch asserted := boxed.(type) {
	case []float64:
		total := asserted[0]
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkTypeSwitchAliasOverwriteIsSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	var boxed any = weights
	switch asserted := boxed.(type) {
	case []float64:
		asserted[0] = 1
	}
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkTypeSwitchDefaultCarriesDynamicType(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	var boxed any = weights
	switch asserted := boxed.(type) {
	case string:
		return
	default:
		total := asserted.([]float64)[0]
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func BenchmarkTypeSwitchMultiCaseCarriesInterfaceValue(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	var boxed any = weights
	switch asserted := boxed.(type) {
	case []float64, []float32:
		total := asserted.([]float64)[0]
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	}
}

func overwriteAndReturn(weights []float64) []float64 {
	weights[0] = 1
	return nil
}

func consumeSlice([]float64) {}

func BenchmarkDeferredArgumentIsEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	defer consumeSlice(overwriteAndReturn(weights))
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func deferredFactory(weights []float64) func() {
	weights[0] = 1
	return func() {}
}

func BenchmarkDeferredFunctionExpressionIsEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	defer deferredFactory(weights)()
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkSendOperandIsEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	ch := make(chan []float64, 1)
	ch <- overwriteAndReturn(weights)
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkSentReferenceEscapes(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	ch := make(chan []float64, 1)
	ch <- weights
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkSelectEvaluatesUnchosenSendOperands(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	var ch chan []float64
	select {
	case ch <- overwriteAndReturn(weights):
	default:
	}
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func channelAndOverwrite(weights []float64) chan int {
	weights[0] = 1
	return nil
}

func BenchmarkSelectEvaluatesUnchosenReceiveOperands(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	select {
	case <-channelAndOverwrite(weights):
	default:
	}
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkStartTimerMethodValue(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	b.StopTimer()
	start := b.StartTimer
	start()
	total := weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkStopTimerMethodValueIsSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	stop := b.StopTimer
	stop()
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkResetTimerMethodValueClearsEarlierGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	total := weights[0]
	if total > 0 {
		sink = total
	}
	reset := b.ResetTimer
	reset()
}

func BenchmarkRunMethodValue(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	run := b.Run
	run("sub", func(b *testing.B) {
		total := weights[0]
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	})
}

func overwriteRunName(weights []float64) string {
	weights[0] = 1
	return "sub"
}

func BenchmarkRunMethodValuePreservesArgumentOrder(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	run := b.Run
	run(overwriteRunName(weights), func(b *testing.B) {
		total := weights[0]
		for b.Loop() {
			if total > 0 {
				sink = total
			}
		}
	})
}

func BenchmarkTestingMethodExpressions(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	(*testing.B).StopTimer(b)
	(*testing.B).StartTimer(b)
	total := weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkRunMethodExpression(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	run := (*testing.B).Run
	run(b, "sub", func(b *testing.B) {
		total := weights[0]
		for b.Loop() {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
				sink = total
			}
		}
	})
}

func BenchmarkTimerReceiverEvaluatedOnce(b *testing.B) {
	weights := []float64{1}
	calls := 0
	receiver := func() *testing.B {
		calls++
		if calls == 1 {
			weights[0] = rand.NormFloat64()
		} else {
			weights[0] = 1
		}
		return b
	}
	receiver().ResetTimer()
	total := weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func BenchmarkTimerReceiverNotEvaluatedTwice(b *testing.B) {
	weights := []float64{1}
	calls := 0
	receiver := func() *testing.B {
		calls++
		if calls == 1 {
			weights[0] = 1
		} else {
			weights[0] = rand.NormFloat64()
		}
		return b
	}
	receiver().ResetTimer()
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func genericCastPair[T ~float32 | ~float64, U any](weights []float64, _ U) T {
	return T(weights[0])
}

func BenchmarkStoredGenericIndexListInstantiation(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	call := genericCastPair[float64, int]
	total := call(weights, 1)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func returnGenericCast() func([]float64, int) float64 {
	return genericCastPair[float64, int]
}

func BenchmarkReturnedGenericInstantiation(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	call := returnGenericCast()
	total := call(weights, 1)
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

type sliceHolder struct{ Weights []float64 }

func echoSliceHolder(holder sliceHolder) sliceHolder { return holder }

func BenchmarkSelectorCallResult(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	total := echoSliceHolder(sliceHolder{Weights: weights}).Weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

type scalarHolder struct{ Value float64 }

func overwriteAndHolder(weights []float64) scalarHolder {
	weights[0] = 1
	return scalarHolder{}
}

func BenchmarkSelectorBaseEffectIsEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	_ = overwriteAndHolder(weights).Value
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func overwriteAndPointer(weights []float64) *float64 {
	weights[0] = 1
	return &weights[0]
}

func BenchmarkDereferenceBaseEffectIsEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	_ = *overwriteAndPointer(weights)
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkIndexBaseEvaluatedOnce(b *testing.B) {
	calls := 0
	values := func() []float64 {
		calls++
		if calls == 1 {
			return []float64{rand.NormFloat64()}
		}
		return []float64{1}
	}
	weights := values()[0]
	total := weights
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func overwriteAndKey(weights []float64) int {
	weights[0] = 1
	return 0
}

func BenchmarkMapLiteralKeyIsEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	_ = map[int]int{overwriteAndKey(weights): 0}
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDeleteKeyIsEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	values := map[int]int{0: 0}
	delete(values, overwriteAndKey(weights))
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkCopyOperandsAreEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	copy(make([]float64, 1), overwriteAndReturn(weights))
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func overwriteAndMap(weights []float64) map[int]int {
	weights[0] = 1
	return map[int]int{}
}

func BenchmarkClearOperandIsEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	clear(overwriteAndMap(weights))
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkCompositeElementEvaluatedOnce(b *testing.B) {
	weights := []float64{1}
	calls := 0
	element := func() int {
		calls++
		if calls == 1 {
			weights[0] = 1
		} else {
			weights[0] = rand.NormFloat64()
		}
		return 0
	}
	_ = []int{element()}
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkCompositeElementFirstEvaluationCarriesRisk(b *testing.B) {
	weights := []float64{1}
	calls := 0
	element := func() int {
		calls++
		if calls == 1 {
			weights[0] = rand.NormFloat64()
		} else {
			weights[0] = 1
		}
		return 0
	}
	_ = []int{element()}
	total := weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func overwriteAndBool(weights []float64) bool {
	weights[0] = 1
	return true
}

func BenchmarkLogicalShortCircuitSkipsRightOperand(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	enabled := b.N > 0
	_ = enabled || overwriteAndBool(weights)
	total := weights[0]
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate; timing may bypass the intended hot branch — add a branch-entry counter or explicit positive-input fixture and keep a separate control cell`
			sink = total
		}
	}
}

func centeredReceiver(weights []float64) *rand.Rand {
	weights[0] = 1
	return rand.New(rand.NewSource(1))
}

func BenchmarkCenteredRandomReceiverIsEvaluated(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	_ = centeredReceiver(weights).Float64() - 0.5
	total := weights[0]
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}
