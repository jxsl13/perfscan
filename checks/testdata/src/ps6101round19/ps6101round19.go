package ps6101round19

import (
	"math/rand"
	"testing"
)

var sink float64

func random(weights []float64) { weights[0] = rand.NormFloat64() }
func one(weights []float64)    { weights[0] = 1 }

func deferSharedRandom(weights []float64, choose bool) {
	defer random(weights)
	if choose {
		defer one(weights)
	}
}

func deferSharedSafe(weights []float64, choose bool) {
	defer one(weights)
	if choose {
		defer random(weights)
	}
}

func namedMultiple(weights []float64, choose bool) (result []float64) {
	result = weights
	defer random(weights)
	if choose {
		return
	}
	return result
}

func ordinaryMultiple(weights []float64, choose bool) []float64 {
	defer random(weights)
	if choose {
		return weights
	}
	return weights
}

func branchOnly(weights []float64, choose bool) []float64 {
	if choose {
		defer random(weights)
		return weights
	}
	return weights
}

func directRecover(weights []float64) {
	defer func() { _ = recover() }()
	panic("stop")
	random(weights)
}

func nestedPanic(weights []float64) { panic("stop") }

func recoverNestedCall() {
	func() { _ = recover() }()
}

func ineffectiveRecover(weights []float64) {
	defer recoverNestedCall()
	panic("stop")
	random(weights)
}

func conditionalRecover(weights []float64, choose bool) {
	defer func() {
		if choose {
			_ = recover()
		}
	}()
	panic("stop")
	random(weights)
}

func multipleRecover(weights []float64) {
	defer func() { _ = recover() }()
	defer func() {}()
	panic("stop")
	random(weights)
}

func unreachableRecover(weights []float64) {
	defer random(weights)
	defer func() {
		if false {
			_ = recover()
		}
	}()
	panic("stop")
}

func namedRecover() { _ = recover() }

func recoveredByNamedDefer(weights []float64) {
	defer random(weights)
	defer namedRecover()
	panic("stop")
}

func recoveredDeferredRandom(weights []float64) {
	defer func() { _ = recover() }()
	defer random(weights)
	panic("stop")
}

type setter struct{ value float64 }

func (s setter) applyValue(weights []float64)    { weights[0] = s.value }
func (s *setter) applyPointer(weights []float64) { weights[0] = s.value }

func valueReceiver(weights []float64) {
	s := setter{value: rand.NormFloat64()}
	defer s.applyValue(weights)
	s.value = 1
}

func pointerReceiver(weights []float64) {
	s := setter{value: rand.NormFloat64()}
	defer s.applyPointer(weights)
	s.value = 1
}

func valueMethodValue(weights []float64) {
	s := setter{value: rand.NormFloat64()}
	call := s.applyValue
	defer call(weights)
	s.value = 1
}

func pointerMethodValue(weights []float64) {
	s := setter{value: rand.NormFloat64()}
	call := s.applyPointer
	defer call(weights)
	s.value = 1
}

func pointerMethodExpression(weights []float64) {
	s := setter{value: rand.NormFloat64()}
	defer (*setter).applyPointer(&s, weights)
	s.value = 1
}

func newPointerPointer() *float64 {
	holder := new(*float64)
	*holder = new(float64)
	**holder = rand.NormFloat64()
	return *holder
}

type box struct{ weight *float64 }

type maker struct{}

func fresh[T any]() *T        { return new(T) }
func (maker) fresh() *float64 { return new(float64) }

func closureBindingCapture(a, b []float64) {
	weights := a
	defer func() { weights[0] = rand.NormFloat64() }()
	weights = b
}

func setRandomSnapshot(weights []float64) { weights[0] = rand.NormFloat64() }

func argumentBindingSnapshot(a, b []float64) {
	weights := a
	defer setRandomSnapshot(weights)
	weights = b
}

func returnedStruct() box {
	result := box{weight: new(float64)}
	*result.weight = rand.NormFloat64()
	return result
}

func returnedArray() [1]*float64 {
	result := [1]*float64{new(float64)}
	*result[0] = rand.NormFloat64()
	return result
}

func returnedStructDeferred() box {
	result := box{weight: new(float64)}
	defer func() { *result.weight = rand.NormFloat64() }()
	return result
}

func returnedArrayDeferred() [1]*float64 {
	result := [1]*float64{new(float64)}
	defer func() { *result[0] = rand.NormFloat64() }()
	return result
}

func returnedPointer() *float64 {
	result := new(float64)
	*result = rand.NormFloat64()
	return result
}

func returnedClosure() (*float64, func()) {
	result := new(float64)
	return result, func() { *result = rand.NormFloat64() }
}

func genericDeferred[T ~float64](weights []float64, value T) {
	defer func(v T) { weights[0] = float64(v) }(value)
}

func gate1(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate2(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate3(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate4(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate5(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate6(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate7(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate8(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate9(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate10(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate11(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate12(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate13(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate14(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate15(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate16(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate17(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func gate18(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate19(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate20(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func gate21(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func safeGate(b *testing.B, total float64) {
	for b.Loop() {
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkDeferSharedRandom(b *testing.B) {
	weights := []float64{1}
	deferSharedRandom(weights, b.N > 1)
	gate1(b, weights[0])
}

func BenchmarkDeferSharedSafe(b *testing.B) {
	weights := []float64{1}
	deferSharedSafe(weights, b.N > 1)
	safeGate(b, weights[0])
}

func BenchmarkNamedMultiple(b *testing.B) {
	weights := []float64{1}
	result := namedMultiple(weights, b.N > 1)
	gate2(b, result[0])
}

func BenchmarkOrdinaryMultiple(b *testing.B) {
	weights := []float64{1}
	result := ordinaryMultiple(weights, b.N > 1)
	gate3(b, result[0])
}

func BenchmarkBranchOnly(b *testing.B) {
	weights := []float64{1}
	result := branchOnly(weights, b.N > 1)
	gate4(b, result[0])
}

func BenchmarkRecoveredPanicUnreachable(b *testing.B) {
	weights := []float64{1}
	directRecover(weights)
	safeGate(b, weights[0])
}

func BenchmarkNestedPanicUnreachable(b *testing.B) {
	weights := []float64{1}
	func() {
		defer func() { _ = recover() }()
		nestedPanic(weights)
		random(weights)
	}()
	safeGate(b, weights[0])
}

func BenchmarkIneffectiveRecoverDoesNotReachGate(b *testing.B) {
	weights := []float64{1}
	ineffectiveRecover(weights)
	safeGate(b, weights[0])
}

func BenchmarkConditionalRecoverUnreachable(b *testing.B) {
	weights := []float64{1}
	conditionalRecover(weights, b.N > 1)
	safeGate(b, weights[0])
}

func BenchmarkMultipleRecoverUnreachable(b *testing.B) {
	weights := []float64{1}
	multipleRecover(weights)
	safeGate(b, weights[0])
}

func BenchmarkUnreachableRecoverDoesNotContinue(b *testing.B) {
	weights := []float64{1}
	unreachableRecover(weights)
	safeGate(b, weights[0])
}

func BenchmarkNamedDirectRecover(b *testing.B) {
	weights := []float64{1}
	recoveredByNamedDefer(weights)
	gate21(b, weights[0])
}

func BenchmarkRecoveredDeferredRandom(b *testing.B) {
	weights := []float64{1}
	recoveredDeferredRandom(weights)
	gate5(b, weights[0])
}

func BenchmarkValueReceiverSnapshot(b *testing.B) {
	weights := []float64{1}
	valueReceiver(weights)
	gate6(b, weights[0])
}

func BenchmarkPointerReceiverBinding(b *testing.B) {
	weights := []float64{1}
	pointerReceiver(weights)
	safeGate(b, weights[0])
}

func BenchmarkValueMethodValueSnapshot(b *testing.B) {
	weights := []float64{1}
	valueMethodValue(weights)
	gate7(b, weights[0])
}

func BenchmarkPointerMethodValueBinding(b *testing.B) {
	weights := []float64{1}
	pointerMethodValue(weights)
	safeGate(b, weights[0])
}

func BenchmarkPointerMethodExpression(b *testing.B) {
	weights := []float64{1}
	pointerMethodExpression(weights)
	safeGate(b, weights[0])
}

func BenchmarkNewPointerPointer(b *testing.B) {
	weight := newPointerPointer()
	gate8(b, *weight)
}

func BenchmarkReturnedStruct(b *testing.B) {
	result := returnedStruct()
	gate9(b, *result.weight)
}

func BenchmarkReturnedArray(b *testing.B) {
	result := returnedArray()
	weight := *result[0]
	gate10(b, weight)
}

func BenchmarkReturnedStructDeferred(b *testing.B) {
	result := returnedStructDeferred()
	gate11(b, *result.weight)
}

func BenchmarkReturnedArrayDeferred(b *testing.B) {
	result := returnedArrayDeferred()
	weight := *result[0]
	gate12(b, weight)
}

func BenchmarkReturnedPointer(b *testing.B) {
	weight := returnedPointer()
	gate13(b, *weight)
}

func BenchmarkReturnedClosure(b *testing.B) {
	weight, assign := returnedClosure()
	assign()
	gate14(b, *weight)
}

func BenchmarkGenericDeferred(b *testing.B) {
	weights := []float64{1}
	genericDeferred(weights, rand.NormFloat64())
	gate15(b, weights[0])
}

func BenchmarkClosureBindingCapture(b *testing.B) {
	safe, weights := []float64{1}, []float64{1}
	closureBindingCapture(safe, weights)
	safeGate(b, safe[0])
	gate16(b, weights[0])
}

func BenchmarkArgumentBindingSnapshot(b *testing.B) {
	weights, safe := []float64{1}, []float64{1}
	argumentBindingSnapshot(weights, safe)
	for index := 0; index < 1; index++ {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
		total = safe[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkFreshGeneric(b *testing.B) {
	weight := fresh[float64]()
	*weight = rand.NormFloat64()
	gate18(b, *weight)
}

func BenchmarkFreshMethod(b *testing.B) {
	weight := (maker{}).fresh()
	*weight = rand.NormFloat64()
	gate19(b, *weight)
}

func BenchmarkFreshDirect(b *testing.B) {
	weight := new(float64)
	*weight = rand.NormFloat64()
	gate20(b, *weight)
}

func BenchmarkFreshDistinctControl(b *testing.B) {
	weight := new(float64)
	other := new(float64)
	*weight = rand.NormFloat64()
	safeGate(b, *other)
}
