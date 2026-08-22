package ps6079

import (
	"math"
	"os"
	"sync/atomic"
	"testing"
	"unsafe"

	"ps6079dep"
)

var forceSSMFast = true
var strictExpGlobalValues = []float64{1}
var positiveGlobalValuesFixture = []float64{1}
var retainedFixtureAddress uintptr
var positiveGlobalArrayFixture = [1]float64{1}
var positiveGlobalBackingFixture = []float64{1}
var positiveGlobalAliasFixture = positiveGlobalBackingFixture
var positiveGlobalInitAliasFixture []float64
var positiveGlobalInitOutAliasFixture []float64
var positiveGlobalInitializerSideEffectFixture []float64
var positiveGlobalAsyncInitializerFixture = ps6079dep.PositiveAsync()
var positiveGlobalAsyncPairInitializerFixture, _ = ps6079dep.PositiveAsyncPair()
var positiveGlobalPairAliasFixture, _ = positiveGlobalPairFixture()
var positiveGlobalRangeBackingFixture = []float64{1}
var positiveGlobalRangeAliasFixture []float64
var positiveReadOnlyPackageFixture = []float64{1}
var negativeReadOnlyPackageFixture = []float64{-1}
var positiveReadOnlyPackageScalar = 1.0
var positivePackageMutatedFixture = []float64{1}
var positivePackageCrossBenchmarkFixture = []float64{1}
var positivePackageReentryFixture = []float64{1}
var positivePackageInitMutatedFixture = []float64{1}
var positivePackageInitAliasBackingFixture = []float64{1}
var positivePackageInitAliasFixture = positivePackageInitAliasBackingFixture
var positivePackageAsyncInitFixture = []float64{1}
var positivePackageCallbackFixture = []float64{1}
var mutatePositivePackageCallbackFixture = func() { positivePackageCallbackFixture[0] = -1 }
var positivePackageUnsafeFixture = []float64{1}
var unsafePackageViewSource = []float64{1}
var unsafePackageBitsSource = []float64{1}
var makeUnsafePackageView = unsafePackageView
var unsafeDynamicViewSource = []float64{1}
var unsafeDynamicBitsSource = []float64{1}
var unsafeStoredViewSource = []float64{1}
var unsafeStoredView string
var unsafeInitializerViewSource = []float64{1}
var unsafeInitializerView = unsafe.String(
	(*byte)(unsafe.Pointer(&unsafeInitializerViewSource[0])), unsafe.Sizeof(unsafeInitializerViewSource[0]),
)
var unsafeInitializerBitsSource = []float64{1}
var unsafeInitializerBits = uint64(uintptr(unsafe.Pointer(&unsafeInitializerBitsSource[0])))
var positivePackageExportedAliasBackingFixture = []float64{1}
var PositivePackageExportedAliasFixture = positivePackageExportedAliasBackingFixture
var positiveDormantCallbackBackingFixture = []float64{1}
var positiveDormantCallbackAliasFixture = []float64{1}
var retainedFixtureCallback func()
var preEventPackageCallbackFixture []float64
var preEventPackageCallback = func() { preEventPackageCallbackFixture[0] = -1 }
var positiveDormantCallback = func() {
	positiveDormantCallbackAliasFixture = positiveDormantCallbackBackingFixture
}
var _ = func() int {
	positiveGlobalInitializerSideEffectFixture = positiveGlobalBackingFixture
	return 0
}()

type retainedMethodFixture struct{ values *[]float64 }

func (fixture retainedMethodFixture) mutate() { (*fixture.values)[0] = -1 }

type signPredicate interface {
	AllPositive([]float64) bool
}

type convertedSignPredicate func([]float64) bool

type falseSignPredicate struct{}

func (falseSignPredicate) AllPositive([]float64) bool { return false }

type fixtureFactory interface {
	PositiveValues() []float64
}

type negativeFixtureFactory struct{}

func (negativeFixtureFactory) PositiveValues() []float64 { return []float64{-1} }

type fixtureMutator interface {
	FillPositive([]float64, float64)
}

type noOpFixtureMutator struct{}

func (noOpFixtureMutator) FillPositive([]float64, float64) {}

type fakeBenchmark testing.B

func (*fakeBenchmark) Fail() {}

type boundFillValues []float64

func (boundFillValues) FillPositive(float64) {}

func mutateStrictExpGlobalValues()    { strictExpGlobalValues[0] = -1 }
func positiveGlobalValues() []float64 { return positiveGlobalValuesFixture }
func mutatePositiveGlobalValues()     { positiveGlobalValuesFixture[0] = -1 }
func positiveGlobalFixtureAddress() uintptr {
	return uintptr(unsafe.Pointer(&positiveGlobalValuesFixture[0]))
}

func positiveGlobalPairFixture() ([]float64, error) { return positiveGlobalBackingFixture, nil }

func setPositiveGlobalInitAlias(values *[]float64) { *values = positiveGlobalBackingFixture }

func yieldPositiveGlobalRangeBacking(yield func([]float64) bool) {
	yield(positiveGlobalRangeBackingFixture)
}

func init() {
	positiveGlobalInitAliasFixture = positiveGlobalBackingFixture
	setPositiveGlobalInitAlias(&positiveGlobalInitOutAliasFixture)
	positivePackageInitMutatedFixture[0] = -1
	positivePackageInitAliasFixture[0] = -1
	go func() { positivePackageAsyncInitFixture[0] = -1 }()
	for positiveGlobalRangeAliasFixture = range yieldPositiveGlobalRangeBacking {
		break
	}
}

func mutatePositivePackageFixtureThroughUnsafe() {
	pointer := uintptr(unsafe.Pointer(&positivePackageUnsafeFixture[0]))
	*(*float64)(unsafe.Pointer(pointer)) = -1
}

func positiveGlobalValuesViaIIFE() []float64 {
	var values []float64
	func() { values = positiveGlobalValuesFixture }()
	return values
}

func setPositiveGlobalValues(values *[]float64) { *values = positiveGlobalValuesFixture }

func positiveGlobalValuesViaOutParameter() []float64 {
	var values []float64
	setPositiveGlobalValues(&values)
	return values
}

func yieldPositiveGlobalValues(yield func([]float64) bool) {
	yield(positiveGlobalValuesFixture)
}

func positiveGlobalValuesViaRange() []float64 {
	var values []float64
	for values = range yieldPositiveGlobalValues {
		break
	}
	return values
}

func positiveGlobalValuesViaFunctionAlias() []float64 {
	get := func() []float64 { return positiveGlobalValuesFixture }
	return get()
}

func positiveGlobalValuesViaDefer() (values []float64) {
	defer func() { values = positiveGlobalValuesFixture }()
	return
}

type packageFixtureHolder struct{ values []float64 }

func (holder *packageFixtureHolder) setFromPackage() {
	holder.values = positiveGlobalValuesFixture
}

func positiveGlobalValuesViaMethodReceiver() []float64 {
	var holder packageFixtureHolder
	holder.setFromPackage()
	return holder.values
}

func setPositiveGlobalValuesFrom(values *[]float64, source []float64) { *values = source }

func positiveGlobalValuesViaSourceOutParameter() []float64 {
	var values []float64
	setPositiveGlobalValuesFrom(&values, positiveGlobalValuesFixture)
	return values
}

func freshPositiveValues() []float64 { return []float64{1} }

func positiveGlobalValuesViaConditionalFunction(cond bool) []float64 {
	get := positiveGlobalValues
	if cond {
		get = freshPositiveValues
	}
	return get()
}

type packageFixtureGetter struct{ get func() []float64 }

func positiveGlobalValuesViaAggregateFunction() []float64 {
	holder := packageFixtureGetter{get: func() []float64 { return positiveGlobalValuesFixture }}
	return holder.get()
}

type packageFixtureIterator struct {
	iterate func(func([]float64) bool)
}

func positiveGlobalValuesViaAggregateRange() []float64 {
	holder := packageFixtureIterator{
		iterate: func(yield func([]float64) bool) { yield(positiveGlobalValuesFixture) },
	}
	var values []float64
	for values = range holder.iterate {
		break
	}
	return values
}

func positiveGlobalValuesViaRecursiveFunction() []float64 {
	var recur func(int)
	recur = func(depth int) {
		if depth > 0 {
			recur(depth - 1)
		}
	}
	recur(0)
	return positiveGlobalValuesFixture
}

func withPositiveGlobalValues(yield func([]float64)) { yield(positiveGlobalValuesFixture) }

func positiveGlobalValuesViaCallback() []float64 {
	var values []float64
	withPositiveGlobalValues(func(source []float64) { values = source })
	return values
}

var setPositiveGlobalValuesOpaque = func(values *[]float64) {
	*values = positiveGlobalValuesFixture
}

func positiveGlobalValuesViaOpaqueOutParameter() []float64 {
	var values []float64
	setPositiveGlobalValuesOpaque(&values)
	return values
}

func positiveGlobalValuesViaRangeRebind() []float64 {
	for _, positiveGlobalValuesFixture = range [][]float64{{-1}} {
	}
	return positiveGlobalValuesFixture
}

func mutatePositiveGlobalValuesIterator(yield func(int) bool) {
	positiveGlobalValuesFixture[0] = -1
	yield(0)
}

func positiveGlobalValuesViaMutatingRange() []float64 {
	for range mutatePositiveGlobalValuesIterator {
	}
	return positiveGlobalValuesFixture
}

func launchPositiveGlobalValues() []float64 {
	go func() { positiveGlobalValuesFixture[0] = -1 }()
	return positiveGlobalValuesFixture
}

func positiveGlobalValuesViaLaunchingFunctionAlias() []float64 {
	launch := launchPositiveGlobalValues
	return launch()
}

func positiveFreshAppendValues() []float64 { return append([]float64(nil), 1) }

func positiveGlobalAliasValues() []float64     { return positiveGlobalAliasFixture }
func positiveGlobalInitAliasValues() []float64 { return positiveGlobalInitAliasFixture }
func positiveGlobalInitOutAliasValues() []float64 {
	return positiveGlobalInitOutAliasFixture
}
func positiveGlobalInitializerSideEffectValues() []float64 {
	return positiveGlobalInitializerSideEffectFixture
}
func positiveGlobalAsyncInitializerValues() []float64 {
	return positiveGlobalAsyncInitializerFixture
}
func positiveGlobalAsyncPairInitializerValues() []float64 {
	return positiveGlobalAsyncPairInitializerFixture
}
func positiveGlobalRangeAliasValues() []float64     { return positiveGlobalRangeAliasFixture }
func positiveDormantCallbackAliasValues() []float64 { return positiveDormantCallbackAliasFixture }
func positiveGlobalPairAliasValues() []float64      { return positiveGlobalPairAliasFixture }

func positiveIdentity(first, _ []float64) []float64 { return first }

func mutateAndReturnPositiveGlobalValues() []float64 {
	positiveGlobalValuesFixture[0] = -1
	return positiveGlobalValuesFixture
}

func positiveGlobalArrayValues() []float64 { return positiveGlobalArrayFixture[:] }
func mutatePositiveGlobalArray()           { positiveGlobalArrayFixture[0] = -1 }

func skipBenchmark(b *testing.B) { b.SkipNow() }

func allNonNegative(values []float64) bool {
	for _, value := range values {
		if value < 0 {
			return false
		}
	}
	return true
}

func allNonPositive(values []float64) bool {
	for _, value := range values {
		if value > 0 {
			return false
		}
	}
	return true
}

func allPositive(values []float64) bool {
	for _, value := range values {
		if value <= 0 {
			return false
		}
	}
	return true
}

func allPositiveMap(values map[int]float64) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if value <= 0 {
			return false
		}
	}
	return true
}

func randomF64Tensor() []float64              { return []float64{-1, 1} }
func makeRandomTensor() []float64             { return []float64{-1, 1} }
func unknownTensor() []float64                { return []float64{-1, 1} }
func nonNegativeTensor() []float64            { return []float64{0, 1} }
func nonPositiveTensor() []float64            { return []float64{0, -1} }
func positiveMambaDelta() []float64           { return []float64{1, 2} }
func negativeMambaDecay() []float64           { return []float64{-1, -2} }
func negativeUint() uint                      { return ^uint(0) }
func positiveMap() map[int]float64            { return map[int]float64{1: 1} }
func fillRandom([]float64)                    {}
func fillPositive([]float64, float64)         {}
func fillNegativeUint([]uint)                 {}
func fillNegativeAny(any)                     {}
func touch([]float64)                         {}
func touchPointerMap(map[*[]float64]struct{}) {}
func mutateScalar(*float64)                   {}
func mutateUnsafeFixturePointer(pointer unsafe.Pointer) {
	*(*float64)(pointer) = -1
}
func setUnsafeFixturePointer(destination *unsafe.Pointer, source unsafe.Pointer) {
	*destination = source
}
func setIndependentUnsafeFixturePointer(destination *unsafe.Pointer, _ unsafe.Pointer) {
	independent := []float64{-1}
	*destination = unsafe.Pointer(&independent[0])
}
func identityUnsafeFixturePointer(pointer unsafe.Pointer) unsafe.Pointer { return pointer }
func identityFixtureAddress(address uintptr) uintptr                     { return address }
func pairFixtureAddress(address uintptr) (uintptr, bool)                 { return address, true }
func negativeFromFixtureAddress(uintptr) float64                         { return -1 }
func negativeAnyFromFixtureAddress(uintptr) any                          { return float64(-1) }
func negativeAnyFromFixturePointer(unsafe.Pointer) any                   { return float64(-1) }
func unsafeFixtureView(values []float64) string {
	return unsafe.String((*byte)(unsafe.Pointer(&values[0])), unsafe.Sizeof(values[0]))
}
func unsafeFixtureBits(values []float64) uint64 {
	return uint64(uintptr(unsafe.Pointer(&values[0])))
}
func fixtureLength(values []float64) int { return len(values) }
func unsafeFixtureViewPair(values []float64) (string, bool) {
	return unsafeFixtureView(values), true
}
func unsafeFixtureBitsPair(values []float64) (uint64, bool) {
	return unsafeFixtureBits(values), true
}
func unsafePackageView() string {
	return unsafe.String(
		(*byte)(unsafe.Pointer(&unsafePackageViewSource[0])), unsafe.Sizeof(unsafePackageViewSource[0]),
	)
}
func unsafePackageBits() uint64 {
	return uint64(uintptr(unsafe.Pointer(&unsafePackageBitsSource[0])))
}
func wrappedUnsafePackageView() string {
	makeView := unsafePackageView
	return makeView()
}
func wrappedPackageFunctionUnsafeView() string { return makeUnsafePackageView() }
func storeUnsafePackageView() {
	unsafeStoredView = unsafe.String(
		(*byte)(unsafe.Pointer(&unsafeStoredViewSource[0])), unsafe.Sizeof(unsafeStoredViewSource[0]),
	)
}
func fillPositiveFrom([]float64, []float64)    {}
func fillPositiveWithScale(float64, []float64) {}
func identity(values []float64) []float64      { return values }
func pair(values []float64) ([]float64, bool)  { return values, true }
func invokeCallback(callback func())           { callback() }
func invokeAny(callback any)                   { callback.(func())() }
func mutateAny(value any)                      { value.([]float64)[0] = -1 }
func ssmNEON([]float64, []float64)             {}
func ssmScanDRangeScalar([]float64, []float64) {}

type unsafePointerObserver struct{ values []float64 }

func (*unsafePointerObserver) Observe([]float64) {}

type unsafeFixtureViewer interface{ View() string }

type unsafeFixtureViewerImpl struct{}

func (unsafeFixtureViewerImpl) View() string {
	return unsafe.String(
		(*byte)(unsafe.Pointer(&unsafeDynamicViewSource[0])), unsafe.Sizeof(unsafeDynamicViewSource[0]),
	)
}

type unsafeFixtureBitViewer interface{ Bits() uint64 }

type unsafeFixtureBitViewerImpl struct{}

func (unsafeFixtureBitViewerImpl) Bits() uint64 {
	return uint64(uintptr(unsafe.Pointer(&unsafeDynamicBitsSource[0])))
}

type unsafeFixturePairViewer interface{ Pair() (string, bool) }

type unsafeFixturePairViewerImpl struct{}

func (unsafeFixturePairViewerImpl) Pair() (string, bool) {
	return unsafe.String(
		(*byte)(unsafe.Pointer(&unsafeDynamicViewSource[0])), unsafe.Sizeof(unsafeDynamicViewSource[0]),
	), true
}

type unsafeFixtureViewSetter interface{ Set(*string) }

type unsafeFixtureViewSetterImpl struct{}

func (unsafeFixtureViewSetterImpl) Set(destination *string) {
	*destination = unsafe.String(
		(*byte)(unsafe.Pointer(&unsafeDynamicViewSource[0])), unsafe.Sizeof(unsafeDynamicViewSource[0]),
	)
}

var retainedFixture []float64

func retainFixture(values []float64) { retainedFixture = values }

func receiveFixture(values <-chan []float64) []float64 { return <-values }

func receiveFixtureByRange(values <-chan []float64) []float64 {
	for value := range values {
		return value
	}
	return nil
}

func receiveFixtureWrapper(values <-chan []float64) []float64 {
	return receiveFixtureByRange(values)
}

func launchFixtureCallback(callback func()) { go callback() }

type fixtureRunner struct{ callback func() }

func (runner fixtureRunner) Run() { runner.callback() }

func receiveFixtureHolder(values <-chan []float64) fixtureHolder {
	return fixtureHolder{values: receiveFixture(values)}
}

func launchFixtureMutator(values []float64, start <-chan struct{}, done chan<- struct{}) {
	go func() {
		<-start
		values[0] = -1
		close(done)
	}()
}

func launchFixtureMutatorWrapper(values []float64, start <-chan struct{}, done chan<- struct{}) {
	launchFixtureMutator(values, start, done)
}

func ssm(delta, decay []float64) {
	if forceSSMFast && allNonNegative(delta) && allNonPositive(decay) {
		ssmNEON(delta, decay)
		ssmFastPathHits.Add(1)
	} else {
		ssmScanDRangeScalar(delta, decay)
	}
}

func initAsyncSSMNEON([]float64, []float64)   {}
func initAsyncSSMScalar([]float64, []float64) {}

func initAsyncSSM(delta, decay []float64) {
	if allNonNegative(delta) && allNonPositive(decay) {
		initAsyncSSMNEON(delta, decay)
		initAsyncSSMFastPathHits.Add(1)
	} else {
		initAsyncSSMScalar(delta, decay)
	}
}

func ssmWithForgedContributorCounter(delta, decay []float64) {
	ssmFastPathHits.Add(1)
	if forceSSMFast && allNonNegative(delta) && allNonPositive(decay) {
		ssmNEON(delta, decay)
	} else {
		ssmScanDRangeScalar(delta, decay)
	}
}

func ssmWithReadOnlyContributorCounter(delta, decay []float64) {
	if forceSSMFast && allNonNegative(delta) && allNonPositive(decay) {
		ssmNEON(delta, decay)
		_ = ssmFastPathHits.Load()
	} else {
		ssmScanDRangeScalar(delta, decay)
	}
}

func ssmWithZeroContributorCounter(delta, decay []float64) {
	if forceSSMFast && allNonNegative(delta) && allNonPositive(decay) {
		ssmNEON(delta, decay)
		ssmFastPathHits.Add(0)
	} else {
		ssmScanDRangeScalar(delta, decay)
	}
}

func ssmWithGotoContributorCounter(delta, decay []float64) {
	if !allNonNegative(delta) || !allNonPositive(decay) {
		ssmScanDRangeScalar(delta, decay)
		goto done
	}
	ssmNEON(delta, decay)
done:
	ssmFastPathHits.Add(1)
}

func ssmWithNestedGotoContributorCounter(delta, decay []float64) {
	if !allNonNegative(delta) || !allNonPositive(decay) {
		ssmScanDRangeScalar(delta, decay)
		if len(delta) != 0 {
			goto done
		}
		return
	}
	ssmNEON(delta, decay)
done:
	ssmFastPathHits.Add(1)
}

func publicSSM(delta, decay []float64) {
	ssm(delta, decay)
}

func ssmWithValidSibling(delta, decay []float64) {
	ssm(positiveMambaDelta(), negativeMambaDecay())
	ssm(delta, decay)
}

func ssmEncodeNEON([]float64)   {}
func ssmEncodeScalar([]float64) {}
func ssmDecodeNEON([]float64)   {}
func ssmDecodeScalar([]float64) {}

func ssmEncode(values []float64) {
	if allPositive(values) {
		ssmEncodeNEON(values)
	} else {
		ssmEncodeScalar(values)
	}
}

func ssmDecode(values []float64) {
	if allNonPositive(values) {
		ssmDecodeNEON(values)
		ssmDecodeFastPathHits.Add(1)
	} else {
		ssmDecodeScalar(values)
	}
}

func encodeDecode(values []float64) {
	ssmEncode(values)
	ssmDecode(values)
}

func expAVX([]float64)    {}
func expScalar([]float64) {}

func strictExp(values []float64) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
}

func opaquePredicateExp(values []float64) {
	positive := func([]float64) bool { return false }
	if positive(values) {
		expAVX(values)
	} else {
		expScalar(values)
	}
}

func aliasedPredicateExp(values []float64) {
	positive := allPositive
	if positive(values) {
		expAVX(values)
	} else {
		expScalar(values)
	}
}

func convertedPredicateExp(values []float64) {
	positive := convertedSignPredicate(allPositive)
	if positive(values) {
		expAVX(values)
	} else {
		expScalar(values)
	}
}

func dynamicallyDispatchedPredicateExp(predicate signPredicate, values []float64) {
	if predicate.AllPositive(values) {
		expAVX(values)
	} else {
		expScalar(values)
	}
}

func capturedReassignedPredicateExp(values []float64) {
	positive := allPositive
	func() { positive = func([]float64) bool { return false } }()
	if positive(values) {
		expAVX(values)
	} else {
		expScalar(values)
	}
}

func mapAVX(map[int]float64)    {}
func mapScalar(map[int]float64) {}

func mapGate(values map[int]float64) {
	if allPositiveMap(values) {
		mapAVX(values)
	} else {
		mapScalar(values)
	}
}

func dualGuardExp(primary, sibling []float64) {
	if allPositive(primary) {
		expAVX(primary)
		expFastPathHits.Add(1)
	} else {
		expScalar(primary)
	}
	if allPositive(sibling) {
		expAVX(sibling)
		expFastPathHits.Add(1)
	} else {
		expScalar(sibling)
	}
}

func countedExpA(values []float64) {
	if allPositive(values) {
		expAVX(values)
		expFastPathHits.Add(1)
	} else {
		expScalar(values)
	}
}

func countedExpB(values []float64) {
	if allPositive(values) {
		expAVX(values)
		expFastPathHits.Add(1)
	} else {
		expScalar(values)
	}
}

func expFunctionsWithSharedCounter(primary, sibling []float64) {
	countedExpA(primary)
	countedExpB(sibling)
}

func mutatingStrictExp(values []float64) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	alias := values
	alias[0] = -1
}

func readOnlyAliasStrictExp(values []float64) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	alias := values
	_ = len(alias)
}

func returnedAliasMutatingStrictExp(values []float64) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	alias := identity(values)
	alias[0] = -1
}

type fixtureHolder struct {
	values []float64
}

func holdFixture(values []float64) fixtureHolder {
	return fixtureHolder{values: values}
}

func pairFixtureHolder(holder fixtureHolder) (fixtureHolder, bool) {
	return holder, true
}

func returnedAggregateAliasMutatingStrictExp(values []float64) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	holder := holdFixture(values)
	holder.values[0] = -1
}

func conditionalAliasMutatingStrictExp(values, other []float64, useOther bool) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	alias := values
	if useOther {
		alias = other
	}
	alias[0] = -1
}

func loopAliasMutatingStrictExp(values, other []float64, useOther bool) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	alias := values
	for useOther {
		alias = other
		break
	}
	alias[0] = -1
}

func aggregateAliasMutatingStrictExp(values []float64) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	holder := struct{ value []float64 }{value: values}
	holder.value[0] = -1
}

func mutateAggregateHolder(holder struct{ value []float64 }) {
	holder.value[0] = -1
}

func aggregateCallMutatingStrictExp(values []float64) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	mutateAggregateHolder(struct{ value []float64 }{value: values})
}

func unconditionalIfInitRebindStrictExp(values, other []float64, useOther bool) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	alias := values
	if alias = other; useOther {
		_ = len(alias)
	}
	alias[0] = -1
}

func conversionMutatingStrictExp(values []float64) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
	mutateAny(any(values))
}

func allRowsPositive(values [][]float64) bool {
	for _, row := range values {
		if !allPositive(row) {
			return false
		}
	}
	return true
}

func positiveRows() [][]float64 { return [][]float64{{1, 2}} }
func rowsAVX([][]float64)       {}
func rowsScalar([][]float64)    {}

func rangeMutatingStrictExp(values [][]float64) {
	if !allRowsPositive(values) {
		rowsScalar(values)
		return
	}
	rowsAVX(values)
	for _, row := range values {
		row[0] = -1
	}
}

func exp(values []float64) {
	if !allPositive(values) {
		expScalar(values)
		return
	}
	expAVX(values)
}

func valueAVX(float64)    {}
func valueScalar(float64) {}

func scalarGate(value float64) {
	if value > 0 {
		valueAVX(value)
	} else {
		valueScalar(value)
	}
}

func unsignedNegativeGate(value uint) {
	if value < 0 {
		valueAVX(float64(value))
	} else {
		valueScalar(float64(value))
	}
}

func allNegativeUint(values []uint) bool {
	for _, value := range values {
		if value >= 0 {
			return false
		}
	}
	return true
}

func unsignedNegativeSliceGate(values []uint) {
	if allNegativeUint(values) {
		valueAVX(float64(values[0]))
	} else {
		valueScalar(float64(values[0]))
	}
}

type unsignedFixture struct{ value uint }

func negativeUnsignedFixtures() []unsignedFixture {
	return []unsignedFixture{{value: ^uint(0)}}
}

func allNegativeUnsignedFixtures(values []unsignedFixture) bool {
	for _, value := range values {
		if value.value >= 0 {
			return false
		}
	}
	return true
}

func unsignedStructNegativeGate(values []unsignedFixture) {
	if allNegativeUnsignedFixtures(values) {
		valueAVX(float64(values[0].value))
	} else {
		valueScalar(float64(values[0].value))
	}
}

func firstPositive(values []float64) {
	if values[0] > 0 {
		expAVX(values)
	} else {
		expScalar(values)
	}
}

func unknownPair() (float64, error) { return -1, nil }

func BenchmarkSSMRandom(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMRandom reaches ssm through ssm but its fixture does not prove guarded fast-path condition.*delta.*requires >= 0.*decay.*requires <= 0.*optimized target is ssmNEON.*ssmScanDRangeScalar`
		ssm(delta, decay)
	}
}

func BenchmarkSSMViaWrapper(b *testing.B) {
	delta := unknownTensor()
	decay := unknownTensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMViaWrapper reaches ssm through publicSSM -> ssm but its fixture does not prove guarded fast-path condition`
		publicSSM(delta, decay)
	}
}

func BenchmarkSSMForceToggleIsNotEvidence(b *testing.B) {
	forceFastPath := true
	_ = forceFastPath
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMForceToggleIsNotEvidence reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
}

func BenchmarkSSMIndexedMutation(b *testing.B) {
	delta := make([]float64, 8)
	decay := make([]float64, 8)
	delta[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMIndexedMutation reaches ssm.*delta.*indexed fixture write is of unknown sign`
		ssm(delta, decay)
	}
}

func BenchmarkSSMPartialClearIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := negativeMambaDecay()
	clear(delta[1:])
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPartialClearIsNotProof reaches ssm.*delta.*partial clear with unproven full destination coverage is of unknown sign`
		ssm(delta, decay)
	}
}

func BenchmarkSSMFullSliceClear(b *testing.B) {
	delta := randomF64Tensor()
	decay := negativeMambaDecay()
	clear(delta[:])
	for b.Loop() {
		ssm(delta, decay)
	}
}

func BenchmarkSSMAsyncClearIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := negativeMambaDecay()
	go clear(delta)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMAsyncClearIsNotProof reaches ssm.*delta.*asynchronous clear is of unknown sign`
		ssm(delta, decay)
	}
}

func BenchmarkSSMPartialSliceAliasClearIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := negativeMambaDecay()
	tail := delta[1:]
	clear(tail)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPartialSliceAliasClearIsNotProof reaches ssm.*delta.*partial clear with unproven full destination coverage is of unknown sign`
		ssm(delta, decay)
	}
}

func BenchmarkSSMWholeSliceAliasClear(b *testing.B) {
	delta := randomF64Tensor()
	decay := negativeMambaDecay()
	alias := delta
	clear(alias)
	for b.Loop() {
		ssm(delta, decay)
	}
}

func BenchmarkSSMPartialSliceAggregateClearIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := negativeMambaDecay()
	holder := struct{ tail []float64 }{tail: delta[1:]}
	clear(holder.tail)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPartialSliceAggregateClearIsNotProof reaches ssm.*delta.*partial clear with unproven full destination coverage is of unknown sign`
		ssm(delta, decay)
	}
}

func BenchmarkSSMWholeSliceAggregateClear(b *testing.B) {
	delta := randomF64Tensor()
	decay := negativeMambaDecay()
	holder := struct{ values []float64 }{values: delta}
	clear(holder.values)
	for b.Loop() {
		ssm(delta, decay)
	}
}

func BenchmarkStrictExpRandom(b *testing.B) {
	values := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRandom reaches strictExp.*requires > 0.*optimized target is expAVX.*expScalar`
		strictExp(values)
	}
}

func BenchmarkUnsignedNegativeConstructorIsNotProof(b *testing.B) {
	for b.Loop() {
		// want +1 `benchmark BenchmarkUnsignedNegativeConstructorIsNotProof reaches unsignedNegativeGate.*requires < 0.*is of unknown sign`
		unsignedNegativeGate(negativeUint())
	}
}

func BenchmarkUnsignedNegativeMutatorIsNotProof(b *testing.B) {
	values := make([]uint, 1)
	fillNegativeUint(values)
	for b.Loop() {
		// want +1 `benchmark BenchmarkUnsignedNegativeMutatorIsNotProof reaches unsignedNegativeSliceGate.*requires < 0.*is of unknown sign`
		unsignedNegativeSliceGate(values)
	}
}

func BenchmarkInterfaceErasedUnsignedNegativeMutatorIsNotProof(b *testing.B) {
	values := make([]uint, 1)
	var boxed any = values
	fillNegativeAny(boxed)
	for b.Loop() {
		// want +1 `benchmark BenchmarkInterfaceErasedUnsignedNegativeMutatorIsNotProof reaches unsignedNegativeSliceGate.*requires < 0.*is of unknown sign`
		unsignedNegativeSliceGate(values)
	}
}

func BenchmarkUnsignedStructNegativeConstructorIsNotProof(b *testing.B) {
	values := negativeUnsignedFixtures()
	for b.Loop() {
		// want +1 `benchmark BenchmarkUnsignedStructNegativeConstructorIsNotProof reaches unsignedStructNegativeGate.*requires < 0.*is of unknown sign`
		unsignedStructNegativeGate(values)
	}
}

func BenchmarkStrictExpReadOnlyPackageFixture(b *testing.B) {
	for b.Loop() {
		strictExp(positiveReadOnlyPackageFixture)
	}
}

func BenchmarkStrictExpNegativePackageFixture(b *testing.B) {
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpNegativePackageFixture reaches strictExp.*requires > 0.*provably negative`
		strictExp(negativeReadOnlyPackageFixture)
	}
}

func BenchmarkScalarReadOnlyPackageFixture(b *testing.B) {
	for b.Loop() {
		scalarGate(positiveReadOnlyPackageScalar)
	}
}

func BenchmarkStrictExpMutatedPackageFixture(b *testing.B) {
	positivePackageMutatedFixture[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMutatedPackageFixture reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positivePackageMutatedFixture)
	}
}

func BenchmarkMutatePackageFixtureForLaterInvocation(*testing.B) {
	positivePackageCrossBenchmarkFixture[0] = -1
}

func BenchmarkStrictExpCrossBenchmarkPackageMutation(b *testing.B) {
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpCrossBenchmarkPackageMutation reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positivePackageCrossBenchmarkFixture)
	}
}

func BenchmarkStrictExpPackageFixtureReentry(b *testing.B) {
	for range b.N {
		// want +1 `benchmark BenchmarkStrictExpPackageFixtureReentry reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positivePackageReentryFixture)
	}
	positivePackageReentryFixture[0] = -1
}

func BenchmarkStrictExpInitMutatedPackageFixture(b *testing.B) {
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpInitMutatedPackageFixture reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positivePackageInitMutatedFixture)
	}
}

func BenchmarkStrictExpInitAliasMutatedPackageFixture(b *testing.B) {
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpInitAliasMutatedPackageFixture reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positivePackageInitAliasBackingFixture)
	}
}

func BenchmarkStrictExpAsyncInitPackageFixture(b *testing.B) {
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAsyncInitPackageFixture reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positivePackageAsyncInitFixture)
	}
}

func BenchmarkMutatePackageFixtureThroughCallback(*testing.B) {
	mutatePositivePackageCallbackFixture()
}

func BenchmarkStrictExpPackageCallbackMutation(b *testing.B) {
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPackageCallbackMutation reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positivePackageCallbackFixture)
	}
}

func BenchmarkMutatePackageFixtureThroughUnsafe(*testing.B) {
	mutatePositivePackageFixtureThroughUnsafe()
}

func BenchmarkStrictExpUnsafePackageMutation(b *testing.B) {
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafePackageMutation reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positivePackageUnsafeFixture)
	}
}

func BenchmarkStrictExpExportedPackageAlias(b *testing.B) {
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpExportedPackageAlias reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positivePackageExportedAliasBackingFixture)
	}
}

func BenchmarkOpaquePredicateNameIsNotTrusted(b *testing.B) {
	values := randomF64Tensor()
	for b.Loop() {
		opaquePredicateExp(values)
	}
}

func BenchmarkAliasedPredicateKeepsGate(b *testing.B) {
	values := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkAliasedPredicateKeepsGate reaches aliasedPredicateExp.*requires > 0.*is of unknown sign`
		aliasedPredicateExp(values)
	}
}

func BenchmarkConvertedPredicateKeepsGate(b *testing.B) {
	values := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkConvertedPredicateKeepsGate reaches convertedPredicateExp.*requires > 0.*is of unknown sign`
		convertedPredicateExp(values)
	}
}

func BenchmarkDynamicPredicateNameIsNotTrusted(b *testing.B) {
	values := randomF64Tensor()
	for b.Loop() {
		dynamicallyDispatchedPredicateExp(falseSignPredicate{}, values)
	}
}

func BenchmarkCapturedReassignedPredicateNameIsNotTrusted(b *testing.B) {
	values := randomF64Tensor()
	for b.Loop() {
		capturedReassignedPredicateExp(values)
	}
}

func BenchmarkStrictExpIndirectConstructorNameIsNotTrusted(b *testing.B) {
	strictExpGlobalValues = positiveMambaDelta()
	positiveValues := func() []float64 { return []float64{-1} }
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIndirectConstructorNameIsNotTrusted reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positiveValues())
	}
}

func BenchmarkStrictExpAliasedConstructorKeepsProof(b *testing.B) {
	makePositive := positiveMambaDelta
	for b.Loop() {
		strictExp(makePositive())
	}
}

func BenchmarkStrictExpDynamicConstructorNameIsNotTrusted(b *testing.B) {
	factory := fixtureFactory(negativeFixtureFactory{})
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDynamicConstructorNameIsNotTrusted reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(factory.PositiveValues())
	}
}

func BenchmarkStrictExpMixedLiteralReturnSourcesAreUnknown(b *testing.B) {
	strictExpGlobalValues = positiveMambaDelta()
	makeValues := func() []float64 {
		if forceSSMFast {
			return strictExpGlobalValues
		}
		return []float64{-1}
	}
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMixedLiteralReturnSourcesAreUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(makeValues())
	}
}

func BenchmarkStrictExpMixedLocalLiteralReturnSourcesAreUnknown(b *testing.B) {
	strictExpGlobalValues = positiveMambaDelta()
	makeValues := func() []float64 {
		values := []float64{-1}
		if forceSSMFast {
			values = strictExpGlobalValues
		}
		return values
	}
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMixedLocalLiteralReturnSourcesAreUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(makeValues())
	}
}

func BenchmarkStrictExpTransformedLiteralReturnSourceIsUnknown(b *testing.B) {
	strictExpGlobalValues = positiveMambaDelta()
	makeValues := func() []float64 {
		return append(strictExpGlobalValues[:0:0], -1)
	}
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpTransformedLiteralReturnSourceIsUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(makeValues())
	}
}

func BenchmarkStrictExpIndirectMutatorNameIsNotTrusted(b *testing.B) {
	values := negativeMambaDecay()
	fillPositive := func([]float64, float64) {}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIndirectMutatorNameIsNotTrusted reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAliasedMutatorKeepsProof(b *testing.B) {
	values := negativeMambaDecay()
	fill := fillPositive
	fill(values, 1)
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpCapturedReassignedMutatorNameIsNotTrusted(b *testing.B) {
	values := negativeMambaDecay()
	fill := fillPositive
	func() { fill = func([]float64, float64) {} }()
	fill(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpCapturedReassignedMutatorNameIsNotTrusted reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDynamicMutatorNameIsNotTrusted(b *testing.B) {
	values := negativeMambaDecay()
	mutator := fixtureMutator(noOpFixtureMutator{})
	mutator.FillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDynamicMutatorNameIsNotTrusted reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpStableBoundMethodMutatorAliasKeepsProof(b *testing.B) {
	values := boundFillValues{-1}
	fill := values.FillPositive
	fill(1)
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpBoundMethodMutatorAliasIsNotProof(b *testing.B) {
	values := boundFillValues{-1}
	fill := values.FillPositive
	values = boundFillValues{-1}
	fill(1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpBoundMethodMutatorAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpConditionalFixture(b *testing.B) {
	values := randomF64Tensor()
	if forceSSMFast {
		values = positiveMambaDelta()
	}
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpConditionalFixture reaches strictExp.*requires > 0.*conditional positiveMambaDelta is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpBypassedPositiveFixture(b *testing.B) {
	values := randomF64Tensor()
	goto workload
	values = positiveMambaDelta()
workload:
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpBypassedPositiveFixture reaches strictExp.*requires > 0.*non-dominating positiveMambaDelta is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpMakeRandom(b *testing.B) {
	values := makeRandomTensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMakeRandom reaches strictExp.*requires > 0.*makeRandomTensor is unrestricted`
		strictExp(values)
	}
}

func BenchmarkStrictExpKeyedHoles(b *testing.B) {
	values := []float64{3: 1}
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpKeyedHoles reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpSliceAliasMutation(b *testing.B) {
	source := positiveMambaDelta()
	values := source
	source[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpSliceAliasMutation reaches strictExp.*requires > 0.*indexed fixture write is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAliasMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	alias := values
	touch(alias)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAliasMutationReachesSource reaches strictExp.*requires > 0.*call to touch may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafePointerAliasMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	raw := unsafe.Pointer(&values[0])
	fillPositive(values, 1)
	*(*float64)(raw) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafePointerAliasMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUintptrAliasMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	address := uintptr(unsafe.Pointer(&values[0]))
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(address)) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUintptrAliasMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafePointerHelperMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	raw := unsafe.Pointer(&values[0])
	fillPositive(values, 1)
	mutateUnsafeFixturePointer(raw)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafePointerHelperMutationReachesSource reaches strictExp.*requires > 0.*call to mutateUnsafeFixturePointer may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpRetainedUnsafePointerRemainsUnknown(b *testing.B) {
	values := positiveMambaDelta()
	raw := unsafe.Pointer(&values[0])
	ps6079dep.RetainAny(raw)
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRetainedUnsafePointerRemainsUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpSentUnsafePointerRemainsUnknown(b *testing.B) {
	values := positiveMambaDelta()
	raw := unsafe.Pointer(&values[0])
	channel := make(chan unsafe.Pointer, 1)
	channel <- raw
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpSentUnsafePointerRemainsUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpNestedUnsafePointerMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(&values[0])) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpNestedUnsafePointerMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpTypedUnsafePointerMutationReachesSource(b *testing.B) {
	type fixturePointer *float64
	type fixtureUnsafePointer unsafe.Pointer
	values := positiveMambaDelta()
	raw := fixtureUnsafePointer(unsafe.Pointer(&values[0]))
	pointer := fixturePointer(raw)
	fillPositive(values, 1)
	*pointer = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpTypedUnsafePointerMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUintptrArithmeticMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	address := uintptr(unsafe.Pointer(&values[0]))
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(address + 0)) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUintptrArithmeticMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeAddMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	raw := unsafe.Pointer(&values[0])
	fillPositive(values, 1)
	*(*float64)(unsafe.Add(raw, 0)) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeAddMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeSliceMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	raw := unsafe.Pointer(&values[0])
	fillPositive(values, 1)
	unsafe.Slice((*float64)(raw), 1)[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeSliceMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	view := unsafe.String((*byte)(unsafe.Pointer(&values[0])), unsafe.Sizeof(values[0]))
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringHelperMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	view := unsafeFixtureView(values)
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringHelperMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringFunctionAliasMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	makeView := unsafeFixtureView
	view := makeView(values)
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringFunctionAliasMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringIIFEMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	view := func() string {
		return unsafe.String((*byte)(unsafe.Pointer(&values[0])), unsafe.Sizeof(values[0]))
	}()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringIIFEMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringTupleMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	view, _ := unsafeFixtureViewPair(values)
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringTupleMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringPackageHelperMutationReachesSource(b *testing.B) {
	values := unsafePackageViewSource
	view := unsafePackageView()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringPackageHelperMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringAliasedPackageHelperWrapperMutationReachesSource(b *testing.B) {
	values := unsafePackageViewSource
	view := wrappedUnsafePackageView()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringAliasedPackageHelperWrapperMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringPackageFunctionWrapperMutationReachesSource(b *testing.B) {
	values := unsafePackageViewSource
	view := wrappedPackageFunctionUnsafeView()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringPackageFunctionWrapperMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringDynamicDispatchMutationReachesSource(b *testing.B) {
	values := unsafeDynamicViewSource
	var viewer unsafeFixtureViewer = unsafeFixtureViewerImpl{}
	view := viewer.View()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringDynamicDispatchMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringDynamicTupleMutationReachesSource(b *testing.B) {
	values := unsafeDynamicViewSource
	var viewer unsafeFixturePairViewer = unsafeFixturePairViewerImpl{}
	view, _ := viewer.Pair()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringDynamicTupleMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringDynamicStoreMutationReachesSource(b *testing.B) {
	values := unsafeDynamicViewSource
	var setter unsafeFixtureViewSetter = unsafeFixtureViewSetterImpl{}
	var view string
	setter.Set(&view)
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(view))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringDynamicStoreMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringHelperStoreMutationReachesSource(b *testing.B) {
	values := unsafeStoredViewSource
	storeUnsafePackageView()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(unsafeStoredView))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringHelperStoreMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeStringPackageInitializerMutationReachesSource(b *testing.B) {
	values := unsafeInitializerViewSource
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(unsafe.StringData(unsafeInitializerView))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeStringPackageInitializerMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafePointerCarrierMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	holder := struct{ raw unsafe.Pointer }{raw: unsafe.Pointer(&values[0])}
	wrapped := any(holder)
	fillPositive(values, 1)
	*(*float64)(wrapped.(struct{ raw unsafe.Pointer }).raw) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafePointerCarrierMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpReturnedUnsafePointerMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	raw := identityUnsafeFixturePointer(unsafe.Pointer(&values[0]))
	fillPositive(values, 1)
	*(*float64)(raw) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpReturnedUnsafePointerMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIIFEUnsafePointerMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	raw := func() unsafe.Pointer {
		return unsafe.Pointer(&values[0])
	}()
	fillPositive(values, 1)
	*(*float64)(raw) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIIFEUnsafePointerMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpReturnedUintptrMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	address := uintptr(unsafe.Pointer(&values[0]))
	forwarded := identityFixtureAddress(address)
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(forwarded)) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpReturnedUintptrMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpClosureAliasUnsafePointerMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	maker := func() unsafe.Pointer {
		return unsafe.Pointer(&values[0])
	}
	raw := maker()
	fillPositive(values, 1)
	*(*float64)(raw) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpClosureAliasUnsafePointerMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkScalarUintptrArgumentDoesNotAliasResult(b *testing.B) {
	values := positiveMambaDelta()
	address := uintptr(unsafe.Pointer(&values[0]))
	negative := negativeFromFixtureAddress(address)
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarUintptrArgumentDoesNotAliasResult reaches scalarGate.*requires > 0`
		scalarGate(negative)
	}
}

func BenchmarkScalarUnsafeCarrierFieldDoesNotAliasResult(b *testing.B) {
	values := positiveMambaDelta()
	holder := struct {
		raw    unsafe.Pointer
		scalar float64
	}{unsafe.Pointer(&values[0]), -1}
	negative := holder.scalar
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarUnsafeCarrierFieldDoesNotAliasResult reaches scalarGate.*requires > 0`
		scalarGate(negative)
	}
}

func BenchmarkScalarUnsafeCarrierInterfaceDoesNotAliasResult(b *testing.B) {
	values := positiveMambaDelta()
	address := uintptr(unsafe.Pointer(&values[0]))
	boxed := negativeAnyFromFixtureAddress(address)
	negative := boxed.(float64)
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarUnsafeCarrierInterfaceDoesNotAliasResult reaches scalarGate.*requires > 0`
		scalarGate(negative)
	}
}

func BenchmarkScalarInlineUnsafeCarrierInterfaceDoesNotAliasResult(b *testing.B) {
	values := positiveMambaDelta()
	boxed := negativeAnyFromFixturePointer(unsafe.Pointer(&values[0]))
	negative := boxed.(float64)
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarInlineUnsafeCarrierInterfaceDoesNotAliasResult reaches scalarGate.*requires > 0`
		scalarGate(negative)
	}
}

func BenchmarkStrictExpUnsafePointerOutParameterMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	var raw unsafe.Pointer
	setUnsafeFixturePointer(&raw, unsafe.Pointer(&values[0]))
	fillPositive(values, 1)
	*(*float64)(raw) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafePointerOutParameterMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafePointerAliasedOutParameterMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	var raw unsafe.Pointer
	destination := &raw
	setUnsafeFixturePointer(destination, unsafe.Pointer(&values[0]))
	fillPositive(values, 1)
	*(*float64)(raw) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafePointerAliasedOutParameterMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafePointerCopyMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	source := []unsafe.Pointer{unsafe.Pointer(&values[0])}
	destination := make([]unsafe.Pointer, 1)
	copy(destination, source)
	fillPositive(values, 1)
	*(*float64)(destination[0]) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafePointerCopyMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUintptrStructMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	holder := struct{ address uintptr }{uintptr(unsafe.Pointer(&values[0]))}
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(holder.address)) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUintptrStructMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIntegerRoundTripMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	bits := uint64(uintptr(unsafe.Pointer(&values[0])))
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(uintptr(bits))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIntegerRoundTripMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIntegerHelperRoundTripMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	bits := unsafeFixtureBits(values)
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(uintptr(bits))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIntegerHelperRoundTripMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIntegerFunctionAliasRoundTripMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	makeBits := unsafeFixtureBits
	bits := makeBits(values)
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(uintptr(bits))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIntegerFunctionAliasRoundTripMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIntegerTupleRoundTripMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	bits, _ := unsafeFixtureBitsPair(values)
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(uintptr(bits))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIntegerTupleRoundTripMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIntegerPackageHelperRoundTripMutationReachesSource(b *testing.B) {
	values := unsafePackageBitsSource
	bits := unsafePackageBits()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(uintptr(bits))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIntegerPackageHelperRoundTripMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIntegerDynamicDispatchRoundTripMutationReachesSource(b *testing.B) {
	values := unsafeDynamicBitsSource
	var viewer unsafeFixtureBitViewer = unsafeFixtureBitViewerImpl{}
	bits := viewer.Bits()
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(uintptr(bits))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIntegerDynamicDispatchRoundTripMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIntegerPackageInitializerRoundTripMutationReachesSource(b *testing.B) {
	values := unsafeInitializerBitsSource
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(uintptr(unsafeInitializerBits))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIntegerPackageInitializerRoundTripMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIntegerUnaryRoundTripMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	bits := uint64(uintptr(unsafe.Pointer(&values[0])))
	flipped := ^bits
	restored := ^flipped
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(uintptr(restored))) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIntegerUnaryRoundTripMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkScalarUnsafeAddressArithmeticDoesNotInheritFixtureState(b *testing.B) {
	values := positiveMambaDelta()
	address := uintptr(unsafe.Pointer(&values[0]))
	zero := float64(address & 0)
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarUnsafeAddressArithmeticDoesNotInheritFixtureState reaches scalarGate.*requires > 0`
		scalarGate(zero)
	}
}

func BenchmarkStrictExpUnsafeBitcastAfterFillRemainsUnknown(b *testing.B) {
	values := positiveMambaDelta()
	fillPositive(values, 1)
	bits := uint64(uintptr(unsafe.Pointer(&values[0])))
	encoded := math.Float64frombits(bits)
	decoded := uintptr(math.Float64bits(encoded))
	*(*float64)(unsafe.Pointer(decoded)) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnsafeBitcastAfterFillRemainsUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUnsafeObservationRemainsProof(b *testing.B) {
	values := positiveMambaDelta()
	_ = unsafe.Sizeof(values)
	_ = unsafe.Sizeof(values[0])
	_ = unsafe.Alignof(values[0])
	fillPositive(values, 1)
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpBuiltinLengthRemainsProof(b *testing.B) {
	values := positiveMambaDelta()
	_ = len(values)
	fillPositive(values, 1)
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpLengthHelperRemainsProof(b *testing.B) {
	values := positiveMambaDelta()
	_ = fixtureLength(values)
	fillPositive(values, 1)
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpMultiSourceUnsafeCarrierRemainsUnknown(b *testing.B) {
	negative := []float64{-1}
	positive := positiveMambaDelta()
	holders := []unsafe.Pointer{unsafe.Pointer(&negative[0]), unsafe.Pointer(&positive[0])}
	fillPositive(positive, 1)
	alias := unsafe.Slice((*float64)(holders[0]), 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMultiSourceUnsafeCarrierRemainsUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(alias)
	}
}

func BenchmarkStrictExpZeroCopyUnsafeCarrierRemainsUnknown(b *testing.B) {
	negative := []float64{-1}
	positive := positiveMambaDelta()
	source := []unsafe.Pointer{unsafe.Pointer(&positive[0])}
	destination := []unsafe.Pointer{unsafe.Pointer(&negative[0])}
	copy(destination[:0], source)
	fillPositive(positive, 1)
	alias := unsafe.Slice((*float64)(destination[0]), 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpZeroCopyUnsafeCarrierRemainsUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(alias)
	}
}

func BenchmarkStrictExpUintptrRangeMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	addresses := []uintptr{uintptr(unsafe.Pointer(&values[0]))}
	for _, address := range addresses {
		fillPositive(values, 1)
		*(*float64)(unsafe.Pointer(address)) = -1
	}
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUintptrRangeMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpUintptrMultiResultMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	address := uintptr(unsafe.Pointer(&values[0]))
	forwarded, _ := pairFixtureAddress(address)
	fillPositive(values, 1)
	*(*float64)(unsafe.Pointer(forwarded)) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUintptrMultiResultMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPackageUintptrHelperMutationReachesSource(b *testing.B) {
	address := positiveGlobalFixtureAddress()
	fillPositive(positiveGlobalValuesFixture, 1)
	*(*float64)(unsafe.Pointer(address)) = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPackageUintptrHelperMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positiveGlobalValuesFixture)
	}
}

func BenchmarkStrictExpRetainedUintptrRemainsUnknown(b *testing.B) {
	values := positiveMambaDelta()
	retainedFixtureAddress = uintptr(unsafe.Pointer(&values[0]))
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRetainedUintptrRemainsUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAtomicPointerMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	var holder atomic.Pointer[float64]
	holder.Store(&values[0])
	fillPositive(values, 1)
	*holder.Load() = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAtomicPointerMutationReachesSource reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAtomicPointerFailedCASRemainsUnknown(b *testing.B) {
	positive := positiveMambaDelta()
	negative := []float64{-1}
	var holder atomic.Pointer[float64]
	holder.Store(&negative[0])
	holder.CompareAndSwap(nil, &positive[0])
	fillPositive(positive, 1)
	loaded := unsafe.Slice(holder.Load(), 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAtomicPointerFailedCASRemainsUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(loaded)
	}
}

func BenchmarkStrictExpPureCarrierObserverDoesNotAliasArgument(b *testing.B) {
	holder := unsafePointerObserver{values: []float64{-1}}
	pointer := &holder
	source := positiveMambaDelta()
	pointer.Observe(source)
	fillPositive(source, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPureCarrierObserverDoesNotAliasArgument reaches strictExp.*requires > 0`
		strictExp(holder.values)
	}
}

func BenchmarkStrictExpIndependentOutParameterDoesNotAliasArgument(b *testing.B) {
	source := positiveMambaDelta()
	var raw unsafe.Pointer
	setIndependentUnsafeFixturePointer(&raw, unsafe.Pointer(&source[0]))
	fillPositive(source, 1)
	independent := unsafe.Slice((*float64)(raw), 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIndependentOutParameterDoesNotAliasArgument reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(independent)
	}
}

func BenchmarkStrictExpGoroutineUnsafePointerRemainsUnknown(b *testing.B) {
	values := positiveMambaDelta()
	raw := unsafe.Pointer(&values[0])
	ready := make(chan struct{})
	go func(pointer unsafe.Pointer) {
		<-ready
		*(*float64)(pointer) = -1
	}(raw)
	fillPositive(values, 1)
	close(ready)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpGoroutineUnsafePointerRemainsUnknown reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpMultiResultAliasMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	alias, _ := pair(values)
	fillPositive(values, 1)
	touch(alias)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMultiResultAliasMutationReachesSource reaches strictExp.*requires > 0.*call to touch may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpMultiResultDeclarationAliasMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	var alias, _ = pair(values)
	fillPositive(values, 1)
	touch(alias)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMultiResultDeclarationAliasMutationReachesSource reaches strictExp.*requires > 0.*call to touch may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpMultiResultAggregateAliasMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	holder := fixtureHolder{values: values}
	alias, _ := pairFixtureHolder(holder)
	fillPositive(holder.values, 1)
	touch(alias.values)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMultiResultAggregateAliasMutationReachesSource reaches strictExp.*requires > 0.*holder.values is of unknown sign`
		strictExp(holder.values)
	}
}

func BenchmarkStrictExpMapKeyAliasMutationReachesSource(b *testing.B) {
	values := positiveMambaDelta()
	holder := map[*[]float64]struct{}{&values: {}}
	fillPositive(values, 1)
	touchPointerMap(holder)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMapKeyAliasMutationReachesSource reaches strictExp.*requires > 0.*call to touchPointerMap may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPartialCopy(b *testing.B) {
	values := make([]float64, 3)
	copy(values, positiveMambaDelta())
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPartialCopy reaches strictExp.*requires > 0.*copy with unproven full destination coverage is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDiscardedAppendMutation(b *testing.B) {
	values := []float64{1}
	_ = append(values[:0], -1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDiscardedAppendMutation reaches strictExp.*requires > 0.*append may mutate fixture backing storage is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPartialSliceFillIsNotProof(b *testing.B) {
	values := randomF64Tensor()
	fillPositive(values[1:], 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPartialSliceFillIsNotProof reaches strictExp.*requires > 0.*partial fixture mutation by fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPartialSliceMemberFillIsNotProof(b *testing.B) {
	values := randomF64Tensor()
	holder := struct{ tail []float64 }{}
	holder.tail = values[1:]
	fillPositive(holder.tail, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPartialSliceMemberFillIsNotProof reaches strictExp.*requires > 0.*partial fixture mutation by fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpFullSliceFill(b *testing.B) {
	values := randomF64Tensor()
	fillPositive(values[:], 1)
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpAsyncFillIsNotProof(b *testing.B) {
	values := randomF64Tensor()
	go fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAsyncFillIsNotProof reaches strictExp.*requires > 0.*asynchronous fixture mutation by fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpEscapedBeforeFillIsNotProof(b *testing.B) {
	values := make([]float64, 1)
	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		<-start
		values[0] = -1
		close(done)
	}()
	fillPositive(values, 1)
	close(start)
	<-done
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpEscapedBeforeFillIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpEscapedBeforeFirstFixtureEventIsNotProof(b *testing.B) {
	var values []float64
	start := make(chan struct{})
	go func() {
		<-start
		values[0] = -1
	}()
	values = []float64{1}
	fillPositive(values, 1)
	close(start)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpEscapedBeforeFirstFixtureEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpFunctionAliasEscapedBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	start := make(chan struct{})
	mutate := func() {
		<-start
		values[0] = -1
	}
	go mutate()
	values = []float64{1}
	fillPositive(values, 1)
	close(start)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpFunctionAliasEscapedBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpCallbackArgumentBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	ps6079dep.SetCallback(func() { values[0] = -1 })
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpCallbackArgumentBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpRetainedCallbackBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	retainedFixtureCallback = func() { values[0] = -1 }
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRetainedCallbackBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpSentCallbackBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	callbacks := make(chan func(), 1)
	mutate := func() { values[0] = -1 }
	callbacks <- mutate
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpSentCallbackBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAggregateCallbackBeforeFirstEventIsNotProof(b *testing.B) {
	type callbackHolder struct{ function func() }
	var values []float64
	holder := callbackHolder{function: func() { values[0] = -1 }}
	ps6079dep.RetainAny(holder)
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAggregateCallbackBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpMethodValueBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	fixture := retainedMethodFixture{values: &values}
	ps6079dep.RetainAny(fixture.mutate)
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMethodValueBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpFunctionRangeBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	for range func(func() bool) {
		go func() { values[0] = -1 }()
	} {
	}
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpFunctionRangeBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPackageCallbackBeforeFirstEventIsNotProof(b *testing.B) {
	go preEventPackageCallback()
	preEventPackageCallbackFixture = []float64{1}
	fillPositive(preEventPackageCallbackFixture, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPackageCallbackBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(preEventPackageCallbackFixture)
	}
}

func BenchmarkStrictExpCopiedCallbackBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	source := []func(){func() { values[0] = -1 }}
	destination := make([]func(), 1)
	copy(destination, source)
	ps6079dep.RetainAny(destination)
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpCopiedCallbackBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpRangedCallbackBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	callbacks := map[string]func(){"mutate": func() { values[0] = -1 }}
	var mutate func()
	for _, mutate = range callbacks {
		break
	}
	go mutate()
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRangedCallbackBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpMultiValueCallbackBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	callbacks := map[string]func(){"mutate": func() { values[0] = -1 }}
	mutate, _ := callbacks["mutate"]
	go mutate()
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMultiValueCallbackBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDiscardedAppendCallbackBeforeFirstEventIsNotProof(b *testing.B) {
	var values []float64
	callbacks := make([]func(), 1)
	_ = append(callbacks[:0], func() { values[0] = -1 })
	ps6079dep.RetainAny(callbacks)
	values = []float64{1}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDiscardedAppendCallbackBeforeFirstEventIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAliasCreatedAfterEscapeIsNotProof(b *testing.B) {
	values := make([]float64, 1)
	start := make(chan struct{})
	done := make(chan struct{})
	go func(input []float64) {
		<-start
		input[0] = -1
		close(done)
	}(values)
	alias := values
	values = make([]float64, 1)
	fillPositive(alias, 1)
	close(start)
	<-done
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAliasCreatedAfterEscapeIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(alias)
	}
}

func BenchmarkStrictExpHelperEscapeIsNotProof(b *testing.B) {
	values := make([]float64, 1)
	start := make(chan struct{})
	done := make(chan struct{})
	launchFixtureMutatorWrapper(values, start, done)
	fillPositive(values, 1)
	close(start)
	<-done
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpHelperEscapeIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpChannelEscapeIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	mutations := make(chan []float64)
	done := make(chan struct{})
	go func() {
		input := <-mutations
		input[0] = -1
		close(done)
	}()
	mutations <- values
	<-done
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpChannelEscapeIsNotProof reaches strictExp.*requires > 0.*channel send may escape fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpChannelReceiveIsNotProof(b *testing.B) {
	values := make([]float64, 1)
	fixtures := make(chan []float64, 1)
	fixtures <- values
	alias := <-fixtures
	fillPositive(alias, 1)
	values[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpChannelReceiveIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(alias)
	}
}

func BenchmarkStrictExpChannelRangeReceiveIsNotProof(b *testing.B) {
	values := make([]float64, 1)
	fixtures := make(chan []float64, 1)
	fixtures <- values
	alias := make([]float64, 1)
	for alias = range fixtures {
		break
	}
	fillPositive(alias, 1)
	values[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpChannelRangeReceiveIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after .*fillPositive is of unknown sign`
		strictExp(alias)
	}
}

func BenchmarkStrictExpHelperRangeReceiveIsNotProof(b *testing.B) {
	values := make([]float64, 1)
	fixtures := make(chan []float64, 1)
	fixtures <- values
	close(fixtures)
	alias := receiveFixtureWrapper(fixtures)
	fillPositive(alias, 1)
	values[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpHelperRangeReceiveIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(alias)
	}
}

func BenchmarkStrictExpNestedHelperReceiveIsNotProof(b *testing.B) {
	values := make([]float64, 1)
	fixtures := make(chan []float64, 1)
	fixtures <- values
	alias := identity(receiveFixtureHolder(fixtures).values)
	fillPositive(alias, 1)
	values[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpNestedHelperReceiveIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(alias)
	}
}

func BenchmarkStrictExpConditionalAliasFillIsNotProof(b *testing.B) {
	values := negativeMambaDecay()
	alias := values
	if forceSSMFast {
		alias = make([]float64, 1)
	}
	fillPositive(alias, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpConditionalAliasFillIsNotProof reaches strictExp.*requires > 0.*partial fixture mutation by fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpParallelAliasSwapIsNotProof(b *testing.B) {
	left := negativeMambaDecay()
	right := negativeMambaDecay()
	left, right = right, left
	fillPositive(left, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpParallelAliasSwapIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(right)
	}
}

func BenchmarkStrictExpRangeAssignmentIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	for _, values = range [][]float64{negativeMambaDecay()} {
		break
	}
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRangeAssignmentIsNotProof reaches strictExp.*requires > 0.*range assignment is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDerivedSelectorAliasIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	alias := struct{ values []float64 }{values: values}.values
	alias[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDerivedSelectorAliasIsNotProof reaches strictExp.*requires > 0.*indexed fixture write is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDerivedIndexAliasIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	alias := [][]float64{values}[0]
	alias[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDerivedIndexAliasIsNotProof reaches strictExp.*requires > 0.*indexed fixture write is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValues()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPackageHelperAliasRemainsProof(b *testing.B) {
	values := positiveGlobalValues()
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpParallelPackageHelperAliasIsNotProof(b *testing.B) {
	values, _ := positiveGlobalValues(), 0
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpParallelPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpIIFEPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaIIFE()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpIIFEPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpOutParameterPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaOutParameter()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpOutParameterPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpRangePackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaRange()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRangePackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpFunctionAliasPackageHelperIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaFunctionAlias()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpFunctionAliasPackageHelperIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDeferredPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaDefer()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDeferredPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPackageArrayHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalArrayValues()
	mutatePositiveGlobalArray()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPackageArrayHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpMethodReceiverPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaMethodReceiver()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMethodReceiverPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpSourceOutParameterPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaSourceOutParameter()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpSourceOutParameterPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpConditionalFunctionPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaConditionalFunction(true)
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpConditionalFunctionPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAggregateFunctionPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaAggregateFunction()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAggregateFunctionPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAggregateRangePackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaAggregateRange()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAggregateRangePackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpRecursiveFunctionPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaRecursiveFunction()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRecursiveFunctionPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpNestedMutatingPackageCallIsNotProof(b *testing.B) {
	values := positiveIdentity(positiveGlobalValues(), mutateAndReturnPositiveGlobalValues())
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpNestedMutatingPackageCallIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpNestedPurePackageCallRemainsProof(b *testing.B) {
	values := positiveIdentity(positiveGlobalValues(), freshPositiveValues())
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpOutParameterPackageHelperAliasRemainsProof(b *testing.B) {
	values := positiveGlobalValuesViaOutParameter()
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpFunctionAliasPackageHelperRemainsProof(b *testing.B) {
	values := positiveGlobalValuesViaFunctionAlias()
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpDeferredPackageHelperAliasRemainsProof(b *testing.B) {
	values := positiveGlobalValuesViaDefer()
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpMethodReceiverPackageHelperAliasRemainsProof(b *testing.B) {
	values := positiveGlobalValuesViaMethodReceiver()
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpCallbackPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaCallback()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpCallbackPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpOpaqueOutParameterPackageHelperAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaOpaqueOutParameter()
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpOpaqueOutParameterPackageHelperAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPackageRangeRebindIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaRangeRebind()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPackageRangeRebindIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpMutatingFunctionRangeIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaMutatingRange()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMutatingFunctionRangeIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpPackageInitializerAliasIsNotProof(b *testing.B) {
	values := positiveGlobalAliasValues()
	positiveGlobalBackingFixture[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPackageInitializerAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpInitAliasIsNotProof(b *testing.B) {
	values := positiveGlobalInitAliasValues()
	positiveGlobalBackingFixture[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpInitAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpInitOutParameterAliasIsNotProof(b *testing.B) {
	values := positiveGlobalInitOutAliasValues()
	positiveGlobalBackingFixture[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpInitOutParameterAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpInitializerSideEffectAliasIsNotProof(b *testing.B) {
	values := positiveGlobalInitializerSideEffectValues()
	positiveGlobalBackingFixture[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpInitializerSideEffectAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAsyncInitializerAliasIsNotProof(b *testing.B) {
	values := positiveGlobalAsyncInitializerValues()
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAsyncInitializerAliasIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAsyncPairInitializerAliasIsNotProof(b *testing.B) {
	values := positiveGlobalAsyncPairInitializerValues()
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAsyncPairInitializerAliasIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpInitFunctionRangeAliasIsNotProof(b *testing.B) {
	values := positiveGlobalRangeAliasValues()
	positiveGlobalRangeBackingFixture[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpInitFunctionRangeAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDormantInitializerCallbackRemainsProof(b *testing.B) {
	values := positiveDormantCallbackAliasValues()
	positiveDormantCallbackBackingFixture[0] = -1
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpPackageMultiResultInitializerAliasIsNotProof(b *testing.B) {
	values := positiveGlobalPairAliasValues()
	positiveGlobalBackingFixture[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpPackageMultiResultInitializerAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpReturnedArgumentRetentionIsNotProof(b *testing.B) {
	values := positiveGlobalValues()
	ps6079dep.Retain(positiveGlobalValues())
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpReturnedArgumentRetentionIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpClearReturnedAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValues()
	clear(positiveGlobalValues())
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpClearReturnedAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpReturnedAliasLHSIsNotProof(b *testing.B) {
	values := positiveGlobalValues()
	positiveGlobalValues()[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpReturnedAliasLHSIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDirectReturnedAliasIsNotProof(b *testing.B) {
	mutatePositiveGlobalValues()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDirectReturnedAliasIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(positiveGlobalValues())
	}
}

func BenchmarkStrictExpLaunchingFunctionAliasIsNotProof(b *testing.B) {
	values := positiveGlobalValuesViaLaunchingFunctionAlias()
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpLaunchingFunctionAliasIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpExternalAsyncResultIsNotProof(b *testing.B) {
	values := ps6079dep.PositiveAsync()
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpExternalAsyncResultIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpFreshBuiltinResultRemainsProof(b *testing.B) {
	values := positiveFreshAppendValues()
	mutatePositiveGlobalValues()
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpAggregateMemberFillIsNotProof(b *testing.B) {
	left := negativeMambaDecay()
	right := negativeMambaDecay()
	holder := struct {
		left  []float64
		right []float64
	}{left: left, right: right}
	fillPositive(holder.left, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAggregateMemberFillIsNotProof reaches strictExp.*requires > 0.*partial fixture mutation by fillPositive is of unknown sign`
		strictExp(right)
	}
}

func BenchmarkStrictExpCallbackMutationIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	invokeCallback(func() { values[0] = -1 })
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpCallbackMutationIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpFunctionReceiverMutationIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	runner := fixtureRunner{callback: func() { values[0] = -1 }}
	runner.Run()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpFunctionReceiverMutationIsNotProof reaches strictExp.*requires > 0.*is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpFunctionRangeMutationIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	for range func(func() bool) {
		values[0] = -1
	} {
	}
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpFunctionRangeMutationIsNotProof reaches strictExp.*requires > 0.*range-over-function iterator may access fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAsyncFunctionRangeFillIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	for range func(func() bool) {
		go func() { values[0] = -1 }()
	} {
	}
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAsyncFunctionRangeFillIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpFunctionRangeAliasIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	alias := []float64(nil)
	iterator := func(yield func([]float64) bool) { yield(values) }
	for alias = range iterator {
		break
	}
	alias[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpFunctionRangeAliasIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after indexed fixture write is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAsyncCallbackFillIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	launchFixtureCallback(func() { values[0] = -1 })
	fillPositive(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAsyncCallbackFillIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpRetainedHelperFillIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	retainFixture(values)
	fillPositive(values, 1)
	retainedFixture[0] = -1
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRetainedHelperFillIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDirectRetentionFillIsNotProof(b *testing.B) {
	values := positiveMambaDelta()
	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		<-start
		retainedFixture[0] = -1
		close(done)
	}()
	retainedFixture = values
	fillPositive(values, 1)
	close(start)
	<-done
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDirectRetentionFillIsNotProof reaches strictExp.*requires > 0.*asynchronously escaped fixture remains unknown after fillPositive is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkMapDeleteIsNotProof(b *testing.B) {
	values := positiveMap()
	delete(values, 1)
	for b.Loop() {
		// want +1 `benchmark BenchmarkMapDeleteIsNotProof reaches mapGate.*requires > 0.*delete may change map predicate is of unknown sign`
		mapGate(values)
	}
}

func BenchmarkMapClearIsNotProof(b *testing.B) {
	values := positiveMap()
	clear(values)
	for b.Loop() {
		// want +1 `benchmark BenchmarkMapClearIsNotProof reaches mapGate.*requires > 0.*clear may change map predicate is of unknown sign`
		mapGate(values)
	}
}

func BenchmarkScalarAbsNaNIsNotProof(b *testing.B) {
	value := math.Abs(math.NaN())
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarAbsNaNIsNotProof reaches scalarGate.*requires > 0.*math.Abs with unproven non-NaN input is of unknown sign`
		scalarGate(value)
	}
}

func BenchmarkScalarAbsKnownValue(b *testing.B) {
	value := math.Abs(-1)
	for b.Loop() {
		scalarGate(value)
	}
}

func BenchmarkStrictExpResolvedHelperMutatesGlobal(b *testing.B) {
	fillPositive(strictExpGlobalValues, 1)
	for b.Loop() {
		mutateStrictExpGlobalValues()
		// want +1 `benchmark BenchmarkStrictExpResolvedHelperMutatesGlobal reaches strictExp.*requires > 0.*call to mutateStrictExpGlobalValues may mutate package fixture is of unknown sign`
		strictExp(strictExpGlobalValues)
	}
}

func BenchmarkStrictExpUnknownMutator(b *testing.B) {
	values := positiveMambaDelta()
	touch(values)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpUnknownMutator reaches strictExp.*requires > 0.*call to touch may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpCapturedClosureMutation(b *testing.B) {
	values := make([]float64, 2)
	fillPositive(values, 1)
	mutate := func() { values[0] = -1 }
	for b.Loop() {
		mutate()
		// want +1 `benchmark BenchmarkStrictExpCapturedClosureMutation reaches strictExp.*requires > 0.*indirect call to mutate may mutate captured fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpLoopCarriedMutation(b *testing.B) {
	values := positiveMambaDelta()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpLoopCarriedMutation reaches strictExp.*requires > 0.*loop-carried call to touch may mutate fixture is of unknown sign`
		strictExp(values)
		touch(values)
	}
}

func BenchmarkStrictExpReturnedAliasMutation(b *testing.B) {
	values := positiveMambaDelta()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpReturnedAliasMutation reaches returnedAliasMutatingStrictExp.*fixture does not prove guarded fast-path condition`
		returnedAliasMutatingStrictExp(values)
	}
}

func BenchmarkStrictExpReturnedAggregateAliasMutation(b *testing.B) {
	values := positiveMambaDelta()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpReturnedAggregateAliasMutation reaches returnedAggregateAliasMutatingStrictExp.*fixture does not prove guarded fast-path condition`
		returnedAggregateAliasMutatingStrictExp(values)
	}
}

func BenchmarkStrictExpMutatorSourceIsNotConstrained(b *testing.B) {
	destination := make([]float64, 2)
	source := randomF64Tensor()
	fillPositiveFrom(destination, source)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMutatorSourceIsNotConstrained reaches strictExp.*requires > 0.*non-destination argument to fillPositiveFrom is of unknown sign`
		strictExp(source)
	}
}

func BenchmarkStrictExpMutatorWithoutFirstDestinationIsUnknown(b *testing.B) {
	values := positiveMambaDelta()
	fillPositiveWithScale(1, values)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpMutatorWithoutFirstDestinationIsUnknown reaches strictExp.*requires > 0.*non-destination argument to fillPositiveWithScale is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpArgumentNameIsNotResultEvidence(b *testing.B) {
	positiveAlias := randomF64Tensor()
	values := identity(positiveAlias)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpArgumentNameIsNotResultEvidence reaches strictExp.*requires > 0.*call to identity may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

type identitySource []float64

func (values identitySource) Identity() []float64 { return values }

func BenchmarkStrictExpReceiverNameIsNotResultEvidence(b *testing.B) {
	positiveAlias := identitySource(randomF64Tensor())
	values := positiveAlias.Identity()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpReceiverNameIsNotResultEvidence reaches strictExp.*requires > 0.*call to positiveAlias.Identity may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDerivedSliceMutation(b *testing.B) {
	values := positiveMambaDelta()
	touch(values[1:])
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpDerivedSliceMutation reaches strictExp.*requires > 0.*call to touch may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpDeferredMutator(b *testing.B) {
	values := positiveMambaDelta()
	defer touch(values)
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpShadowedMake(b *testing.B) {
	make := func() []float64 { return []float64{-1, 1} }
	values := make()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpShadowedMake reaches strictExp.*requires > 0.*indirect call to make may mutate captured fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpShadowedCopy(b *testing.B) {
	copy := func([]float64, []float64) {}
	values := make([]float64, 3)
	copy(values, positiveMambaDelta())
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpShadowedCopy reaches strictExp.*requires > 0.*call to copy may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkScalarCompoundAssignment(b *testing.B) {
	value := float64(1)
	value -= 2
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarCompoundAssignment reaches scalarGate.*requires > 0.*compound assignment is of unknown sign`
		scalarGate(value)
	}
}

func BenchmarkScalarIncrement(b *testing.B) {
	value := float64(-1)
	value++
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarIncrement reaches scalarGate.*requires > 0.*increment or decrement is of unknown sign`
		scalarGate(value)
	}
}

func BenchmarkScalarMultiValueAssignment(b *testing.B) {
	value := float64(1)
	value, _ = unknownPair()
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarMultiValueAssignment reaches scalarGate.*requires > 0.*multi-value assignment is of unknown sign`
		scalarGate(value)
	}
}

func BenchmarkScalarAddressMutation(b *testing.B) {
	value := float64(1)
	mutateScalar(&value)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarAddressMutation reaches scalarGate.*requires > 0.*call to mutateScalar may mutate fixture is of unknown sign`
		scalarGate(value)
	}
}

func BenchmarkScalarPointerAliasMutation(b *testing.B) {
	value := float64(1)
	pointer := &value
	mutateScalar(pointer)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarPointerAliasMutation reaches scalarGate.*requires > 0.*call to mutateScalar may mutate fixture is of unknown sign`
		scalarGate(value)
	}
}

func BenchmarkScalarPointerAliasSurvivesTargetAssignment(b *testing.B) {
	value := float64(0)
	pointer := &value
	value = 1
	mutateScalar(pointer)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarPointerAliasSurvivesTargetAssignment reaches scalarGate.*requires > 0.*call to mutateScalar may mutate fixture is of unknown sign`
		scalarGate(value)
	}
}

func BenchmarkScalarConditionalPointerRebind(b *testing.B) {
	value := float64(0)
	other := float64(0)
	pointer := &value
	value = 1
	if forceSSMFast {
		pointer = &other
	}
	mutateScalar(pointer)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarConditionalPointerRebind reaches scalarGate.*requires > 0.*call to mutateScalar may mutate fixture is of unknown sign`
		scalarGate(value)
	}
}

func BenchmarkIndexedGuardStaysUnmodeled(b *testing.B) {
	values := []float64{1, -1}
	for b.Loop() {
		firstPositive(values)
	}
}

func BenchmarkScalarMutatorOptionIsNotFixture(b *testing.B) {
	values := make([]float64, 4)
	scale := float64(-1)
	fillPositive(values, scale)
	for b.Loop() {
		// want +1 `benchmark BenchmarkScalarMutatorOptionIsNotFixture reaches scalarGate.*requires > 0.*-1 is provably negative`
		scalarGate(scale)
	}
}

func BenchmarkSSMConstrained(b *testing.B) {
	delta := nonNegativeTensor()
	decay := nonPositiveTensor()
	for b.Loop() {
		ssm(delta, decay)
	}
}

func BenchmarkSSMWarmupDoesNotOverrideMeasuredCall(b *testing.B) {
	ssm(randomF64Tensor(), randomF64Tensor())
	delta := nonNegativeTensor()
	decay := nonPositiveTensor()
	for b.Loop() {
		ssm(delta, decay)
	}
}

func BenchmarkSSMBNRandom(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for index := 0; index < b.N; index++ {
		// want +1 `benchmark BenchmarkSSMBNRandom reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
}

func BenchmarkSSMNamedSemanticFixture(b *testing.B) {
	delta := positiveMambaDelta()
	decay := negativeMambaDecay()
	for b.Loop() {
		publicSSM(delta, decay)
	}
}

func BenchmarkSSMForInitializerIsSetup(b *testing.B) {
	randomDelta := randomF64Tensor()
	randomDecay := randomF64Tensor()
	positiveDelta := positiveMambaDelta()
	negativeDecay := negativeMambaDecay()
	for ssm(randomDelta, randomDecay); b.Loop(); {
		ssm(positiveDelta, negativeDecay)
	}
}

type routeCounter struct{ value int }

func (counter *routeCounter) Load() int       { return counter.value }
func (counter *routeCounter) Store(value int) { counter.value = value }
func (counter *routeCounter) Add(value int)   { counter.value += value }
func (counter *routeCounter) Count() int {
	counter.value++
	return counter.value
}

type noOpResetCounter struct{ value int }

func (counter *noOpResetCounter) Load() int     { return counter.value }
func (counter *noOpResetCounter) Store(int)     {}
func (counter *noOpResetCounter) Add(value int) { counter.value += value }

type valueResetCounter struct{ value int }

func (counter *valueResetCounter) Load() int      { return counter.value }
func (counter valueResetCounter) Store(value int) { counter.value = value }
func (counter *valueResetCounter) Add(value int)  { counter.value += value }

type mutatingLoadCounter struct{ value int }

func (counter *mutatingLoadCounter) Load() int {
	counter.value++
	return counter.value
}
func (counter *mutatingLoadCounter) Store(value int) { counter.value = value }
func (counter *mutatingLoadCounter) Add(value int)   { counter.value += value }

type conditionalResetCounter struct{ value atomic.Int64 }

func (counter *conditionalResetCounter) Load() int64 { return counter.value.Load() }
func (counter *conditionalResetCounter) Store(value int64) {
	counter.value.CompareAndSwap(2, value)
}
func (counter *conditionalResetCounter) Add(value int64) { counter.value.Add(value) }

var ssmFastPathHits routeCounter
var ssmFallbackHits routeCounter
var ssmDecodeFastPathHits routeCounter
var otherFastPathHits routeCounter
var unexpectedFastPathHits routeCounter
var expFastPathHits routeCounter
var ssmNoOpResetFastPathHits noOpResetCounter
var ssmValueResetFastPathHits valueResetCounter
var ssmMutatingLoadFastPathHits mutatingLoadCounter
var initAsyncSSMFastPathHits routeCounter
var ssmAtomicFastPathHits atomic.Int64
var ssmConditionalResetFastPathHits conditionalResetCounter
var initAsyncSSMStart = make(chan struct{})
var initAsyncSSMDone = make(chan struct{})

func init() {
	counter := &initAsyncSSMFastPathHits
	var rangedCounter *routeCounter
	for _, rangedCounter = range []*routeCounter{&initAsyncSSMFastPathHits} {
		break
	}
	go func() {
		<-initAsyncSSMStart
		counter.Add(1)
		rangedCounter.Add(1)
		close(initAsyncSSMDone)
	}()
}

func launchSSMCounterWriter(start <-chan struct{}, done chan<- struct{}) {
	go func() {
		<-start
		ssmFastPathHits.Add(1)
		close(done)
	}()
}

func launchSSMCounterWriterWrapper(start <-chan struct{}, done chan<- struct{}) {
	launchSSMCounterWriter(start, done)
}

var globalRouteCallback = func() { ssm(positiveMambaDelta(), negativeMambaDecay()) }
var ssmFastPathHitsAlias = &ssmFastPathHits

func invokeGlobalRouteCallback() { globalRouteCallback() }

func observeRouteReadOnly() { _ = len([]int{1}) }

func panicBeforeRouteAssertion() {
	if forceSSMFast {
		panic("skip route assertion")
	}
}

func addGlobalRouteCounter() { ssmFastPathHits.Add(1) }

func addAliasedGlobalRouteCounter() { ssmFastPathHitsAlias.Add(1) }

func publicSSMWithManualCounter(delta, decay []float64) {
	ssm(delta, decay)
	ssmFastPathHits.Add(1)
}

func returnedSSMCounter() *routeCounter {
	counter := &ssmFastPathHits
	return counter
}

func returnedSSMCounterPair() (*routeCounter, bool) { return &ssmFastPathHits, true }

func ssmWithAtomicCounter(delta, decay []float64) {
	if allNonNegative(delta) && allNonPositive(decay) {
		ssmNEON(delta, decay)
		ssmAtomicFastPathHits.Add(1)
	} else {
		ssmScanDRangeScalar(delta, decay)
	}
}

func ssmWithConditionalResetCounter(delta, decay []float64) {
	if allNonNegative(delta) && allNonPositive(decay) {
		ssmNEON(delta, decay)
		ssmConditionalResetFastPathHits.Add(1)
	} else {
		ssmScanDRangeScalar(delta, decay)
	}
}

func ssmWithNoOpResetCounter(delta, decay []float64) {
	if allNonNegative(delta) && allNonPositive(decay) {
		ssmNEON(delta, decay)
		ssmNoOpResetFastPathHits.Add(1)
	} else {
		ssmScanDRangeScalar(delta, decay)
	}
}

func ssmWithMutatingLoadCounter(delta, decay []float64) {
	if allNonNegative(delta) && allNonPositive(decay) {
		ssmNEON(delta, decay)
		ssmMutatingLoadFastPathHits.Add(1)
	} else {
		ssmScanDRangeScalar(delta, decay)
	}
}

func ssmWithValueResetCounter(delta, decay []float64) {
	if allNonNegative(delta) && allNonPositive(decay) {
		ssmNEON(delta, decay)
		ssmValueResetFastPathHits.Add(1)
	} else {
		ssmScanDRangeScalar(delta, decay)
	}
}

func BenchmarkSSMRouteCounter(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMAtomicRouteCounter(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmAtomicFastPathHits.Store(0)
	for b.Loop() {
		ssmWithAtomicCounter(delta, decay)
	}
	if ssmAtomicFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMNoOpResetIsNotProof(b *testing.B) {
	randomDelta := randomF64Tensor()
	randomDecay := randomF64Tensor()
	ssmWithNoOpResetCounter(positiveMambaDelta(), negativeMambaDecay())
	ssmNoOpResetFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMNoOpResetIsNotProof reaches ssmWithNoOpResetCounter.*fixture does not prove guarded fast-path condition`
		ssmWithNoOpResetCounter(randomDelta, randomDecay)
	}
	if ssmNoOpResetFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMMutatingLoadIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmMutatingLoadFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMMutatingLoadIsNotProof reaches ssmWithMutatingLoadCounter.*fixture does not prove guarded fast-path condition`
		ssmWithMutatingLoadCounter(delta, decay)
	}
	if ssmMutatingLoadFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMValueReceiverResetIsNotProof(b *testing.B) {
	randomDelta := randomF64Tensor()
	randomDecay := randomF64Tensor()
	ssmWithValueResetCounter(positiveMambaDelta(), negativeMambaDecay())
	ssmValueResetFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMValueReceiverResetIsNotProof reaches ssmWithValueResetCounter.*fixture does not prove guarded fast-path condition`
		ssmWithValueResetCounter(randomDelta, randomDecay)
	}
	if ssmValueResetFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMWrapperRouteCounter(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		publicSSM(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMWrapperManualCounterWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMWrapperManualCounterWriteIsNotProof reaches ssm through publicSSMWithManualCounter -> ssm.*fixture does not prove guarded fast-path condition`
		publicSSMWithManualCounter(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMGuardedContributorCounterWriteIsProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMUnconditionalContributorCounterWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMUnconditionalContributorCounterWriteIsNotProof reaches ssmWithForgedContributorCounter.*fixture does not prove guarded fast-path condition`
		ssmWithForgedContributorCounter(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMReadOnlyContributorCounterIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMReadOnlyContributorCounterIsNotProof reaches ssmWithReadOnlyContributorCounter.*fixture does not prove guarded fast-path condition`
		ssmWithReadOnlyContributorCounter(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMZeroContributorCounterIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMZeroContributorCounterIsNotProof reaches ssmWithZeroContributorCounter.*fixture does not prove guarded fast-path condition`
		ssmWithZeroContributorCounter(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMGotoContributorCounterIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMGotoContributorCounterIsNotProof reaches ssmWithGotoContributorCounter.*fixture does not prove guarded fast-path condition`
		ssmWithGotoContributorCounter(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMNestedGotoContributorCounterIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMNestedGotoContributorCounterIsNotProof reaches ssmWithNestedGotoContributorCounter.*fixture does not prove guarded fast-path condition`
		ssmWithNestedGotoContributorCounter(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMRouteCounterWithReadOnlyHelper(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	observeRouteReadOnly()
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMMutatingInterveningCountIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMMutatingInterveningCountIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	_ = ssmFastPathHits.Count()
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMRouteCounterWithValidSiblingIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMRouteCounterWithValidSiblingIsNotProof reaches ssm through ssmWithValidSibling -> ssm.*fixture does not prove guarded fast-path condition`
		ssmWithValidSibling(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkExpCounterSharedBySiblingGatesIsNotProof(b *testing.B) {
	primary := randomF64Tensor()
	sibling := positiveMambaDelta()
	expFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkExpCounterSharedBySiblingGatesIsNotProof reaches dualGuardExp.*primary .*requires > 0 but randomF64Tensor`
		dualGuardExp(primary, sibling)
	}
	if expFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkExpCounterSharedAcrossFunctionsIsNotProof(b *testing.B) {
	primary := randomF64Tensor()
	sibling := positiveMambaDelta()
	expFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkExpCounterSharedAcrossFunctionsIsNotProof reaches countedExpA.*primary.*requires > 0 but randomF64Tensor`
		expFunctionsWithSharedCounter(primary, sibling)
	}
	if expFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMEncodeDecodeCounterConfusionIsNotProof(b *testing.B) {
	values := negativeMambaDecay()
	ssmDecodeFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMEncodeDecodeCounterConfusionIsNotProof reaches ssmEncode through encodeDecode.*requires > 0.*negativeMambaDecay is provably negative`
		encodeDecode(values)
	}
	if ssmDecodeFastPathHits.Load() == 0 {
		b.Fatal("decode fast path was not entered")
	}
}

func BenchmarkSSMPreResetAsyncCounterWriterIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		<-start
		ssmFastPathHits.Add(1)
		close(done)
	}()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPreResetAsyncCounterWriterIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	close(start)
	<-done
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMPreResetHelperCounterWriterIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	start := make(chan struct{})
	done := make(chan struct{})
	launchSSMCounterWriterWrapper(start, done)
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPreResetHelperCounterWriterIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	close(start)
	<-done
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMPreResetReturnedCounterAliasIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	counter := returnedSSMCounter()
	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		<-start
		counter.Add(1)
		close(done)
	}()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPreResetReturnedCounterAliasIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	close(start)
	<-done
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMCompareAndSwapIsNotResetProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmAtomicFastPathHits.Store(1)
	ssmAtomicFastPathHits.CompareAndSwap(2, 0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMCompareAndSwapIsNotResetProof reaches ssmWithAtomicCounter.*fixture does not prove guarded fast-path condition`
		ssmWithAtomicCounter(delta, decay)
	}
	if ssmAtomicFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMCompareAndSwapWrapperIsNotResetProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmConditionalResetFastPathHits.value.Store(1)
	ssmConditionalResetFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMCompareAndSwapWrapperIsNotResetProof reaches ssmWithConditionalResetCounter.*fixture does not prove guarded fast-path condition`
		ssmWithConditionalResetCounter(delta, decay)
	}
	if ssmConditionalResetFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMPreResetReturnedCounterPairAliasIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	counter, _ := returnedSSMCounterPair()
	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		<-start
		counter.Add(1)
		close(done)
	}()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPreResetReturnedCounterPairAliasIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	close(start)
	<-done
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMPreResetCounterChannelSendIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	counters := make(chan *routeCounter, 1)
	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		counter := <-counters
		<-start
		counter.Add(1)
		close(done)
	}()
	counters <- returnedSSMCounter()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPreResetCounterChannelSendIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	close(start)
	<-done
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkInitAsyncSSMCounterWriterIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	initAsyncSSMFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkInitAsyncSSMCounterWriterIsNotProof reaches initAsyncSSM.*fixture does not prove guarded fast-path condition`
		initAsyncSSM(delta, decay)
	}
	close(initAsyncSSMStart)
	<-initAsyncSSMDone
	if initAsyncSSMFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMDirectCounterWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMDirectCounterWriteIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	ssmFastPathHits.Add(1)
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMDirectCounterFieldWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMDirectCounterFieldWriteIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	ssmFastPathHits.value = 1
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMHelperCounterWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMHelperCounterWriteIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	addGlobalRouteCounter()
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMReturnedCounterAliasWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMReturnedCounterAliasWriteIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	counter := returnedSSMCounter()
	counter.Add(1)
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMCounterPointerAliasWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	counter := &ssmFastPathHits
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMCounterPointerAliasWriteIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	counter.Add(1)
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMPackageCounterAliasWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPackageCounterAliasWriteIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	addAliasedGlobalRouteCounter()
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMReboundCounterAliasDoesNotInvalidateProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	counter := &ssmFastPathHits
	counter = &otherFastPathHits
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	counter.Add(1)
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMCounterAggregateAliasWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	holder := struct{ counter *routeCounter }{counter: &ssmFastPathHits}
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMCounterAggregateAliasWriteIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	holder.counter.Add(1)
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMCounterMemberAliasWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	holder := struct{ counter *routeCounter }{}
	holder.counter = &ssmFastPathHits
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMCounterMemberAliasWriteIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	holder.counter.Add(1)
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMReboundCounterAggregateDoesNotInvalidateProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	holder := struct{ counter *routeCounter }{counter: &ssmFastPathHits}
	holder = struct{ counter *routeCounter }{counter: &otherFastPathHits}
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	holder.counter.Add(1)
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMStaleRouteCounterIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMStaleRouteCounterIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMResetBeforeWarmupIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	ssm(positiveMambaDelta(), negativeMambaDecay())
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMResetBeforeWarmupIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMDownstreamWarmupIsNotWrapperProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	ssm(positiveMambaDelta(), negativeMambaDecay())
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMDownstreamWarmupIsNotWrapperProof reaches ssm through publicSSM -> ssm.*fixture does not prove guarded fast-path condition`
		publicSSM(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMPostWorkloadContributorIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMPostWorkloadContributorIsNotProof reaches ssm through publicSSM -> ssm.*fixture does not prove guarded fast-path condition`
		publicSSM(delta, decay)
	}
	ssm(positiveMambaDelta(), negativeMambaDecay())
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMIndirectContributorIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	hit := func() { ssm(positiveMambaDelta(), negativeMambaDecay()) }
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMIndirectContributorIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	hit()
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMCallbackInvokerIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMCallbackInvokerIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	invokeCallback(func() { ssm(positiveMambaDelta(), negativeMambaDecay()) })
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMConvertedCallbackInvokerIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMConvertedCallbackInvokerIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	invokeAny(any(func() { ssm(positiveMambaDelta(), negativeMambaDecay()) }))
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMGlobalCallbackInvokerIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMGlobalCallbackInvokerIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	invokeGlobalRouteCallback()
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMCrossPackageCallbackInvokerIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ps6079dep.SetCallback(func() { ssm(positiveMambaDelta(), negativeMambaDecay()) })
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMCrossPackageCallbackInvokerIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	ps6079dep.Invoke()
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMConditionalResetIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	if forceSSMFast {
		ssmFastPathHits.Store(0)
	}
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMConditionalResetIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMDeferredResetIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	defer ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMDeferredResetIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMWrongCounterPolarity(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMWrongCounterPolarity reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() > 0 {
		b.Fatal("fast path was entered")
	}
}

func BenchmarkSSMReassignedFailureReceiverIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMReassignedFailureReceiverIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	b = new(testing.B)
	if ssmFastPathHits.Load() == 0 {
		b.Error("fast path was not entered")
	}
}

func BenchmarkSSMIndirectlyReassignedFailureReceiverIsNotProof(b *testing.B) {
	defer func() { _ = recover() }()
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMIndirectlyReassignedFailureReceiverIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	pointer := &b
	*pointer = nil
	if ssmFastPathHits.Load() == 0 {
		b.Error("fast path was not entered")
	}
}

func BenchmarkSSMSkipNowBeforeFailureIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMSkipNowBeforeFailureIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.SkipNow()
		b.Error("fast path was not entered")
	}
}

func BenchmarkSSMSkipBeforeFailureIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMSkipBeforeFailureIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Skip("fast path is unavailable")
		b.Error("fast path was not entered")
	}
}

func BenchmarkSSMSkipMethodExpressionBeforeFailureIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMSkipMethodExpressionBeforeFailureIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		(*testing.B).SkipNow(b)
		b.Error("fast path was not entered")
	}
}

func BenchmarkSSMSkipMethodValueBeforeFailureIsNotProof(b *testing.B) {
	skip := b.SkipNow
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMSkipMethodValueBeforeFailureIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		skip()
		b.Error("fast path was not entered")
	}
}

func BenchmarkSSMResolvedSkipHelperBeforeFailureIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMResolvedSkipHelperBeforeFailureIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		skipBenchmark(b)
		b.Error("fast path was not entered")
	}
}

func BenchmarkSSMCrossPackageSkipHelperBeforeFailureIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMCrossPackageSkipHelperBeforeFailureIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		ps6079dep.SkipBenchmark(b)
		b.Error("fast path was not entered")
	}
}

func BenchmarkSSMHelperAfterFailureIsProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Error("fast path was not entered")
		observeRouteReadOnly()
	}
}

func BenchmarkSSMDeferredFailureMayBeBypassed(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMDeferredFailureMayBeBypassed reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		defer b.Error("fast path was not entered")
		os.Exit(0)
	}
}

func BenchmarkSSMAsynchronousFailureMayBeBypassed(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMAsynchronousFailureMayBeBypassed reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		go b.Error("asynchronous failure is not guaranteed")
		os.Exit(0)
		b.Error("fast path was not entered")
	}
}

func BenchmarkSSMMethodExpressionFailureIsProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		(*testing.B).Error(b, "fast path was not entered")
		observeRouteReadOnly()
	}
}

func BenchmarkSSMFailIsProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fail()
	}
}

func BenchmarkSSMFailNowIsProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.FailNow()
	}
}

func BenchmarkSSMMethodExpressionFailIsProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		(*testing.B).Fail(b)
	}
}

func BenchmarkSSMMethodExpressionFailNowIsProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		(*testing.B).FailNow(b)
	}
}

func BenchmarkSSMIdentityConvertedFailureReceiverIsProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		(*testing.B).Fail((*testing.B)(b))
	}
}

func BenchmarkSSMAddressedDereferenceFailureReceiverIsProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		(*testing.B).Fail(&*b)
	}
}

func BenchmarkSSMDeferredFailIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMDeferredFailIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		defer b.Fail()
	}
}

func BenchmarkSSMAsynchronousFailIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMAsynchronousFailIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		go b.Fail()
	}
}

func BenchmarkSSMMethodExpressionFailOnOtherBenchmarkIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	other := new(testing.B)
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMMethodExpressionFailOnOtherBenchmarkIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		(*testing.B).Fail(other)
	}
}

func BenchmarkSSMFakeFailMethodIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMFakeFailMethodIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		(*fakeBenchmark)(b).Fail()
	}
}

func BenchmarkSSMUnsafeFailureReceiverIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMUnsafeFailureReceiverIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		(*testing.B).Fail((*testing.B)(unsafe.Pointer(&b)))
	}
}

func BenchmarkSSMConditionalFailIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMConditionalFailIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 && forceSSMFast {
		b.Fail()
	}
}

func BenchmarkSSMFailureOnlyInElse(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMFailureOnlyInElse reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		_ = delta
	} else {
		b.Fatal("fast path was entered")
	}
}

func BenchmarkSSMFailureOnOtherBenchmarkIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	other := new(testing.B)
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMFailureOnOtherBenchmarkIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		other.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMRecoveredPanicIsNotProof(b *testing.B) {
	defer func() { _ = recover() }()
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMRecoveredPanicIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		panic("fast path was not entered")
	}
}

func BenchmarkSSMRecoveredPanicBypassesAssertion(b *testing.B) {
	defer func() { _ = recover() }()
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMRecoveredPanicBypassesAssertion reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if forceSSMFast {
		panic("skip route assertion")
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMRecoveredHelperPanicBypassesAssertion(b *testing.B) {
	defer func() { _ = recover() }()
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMRecoveredHelperPanicBypassesAssertion reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	panicBeforeRouteAssertion()
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMFallbackCounterIsNotFastProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMFallbackCounterIsNotFastProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFallbackHits.Load() == 0 {
		b.Fatal("fallback path was not entered")
	}
}

func BenchmarkSSMOtherCounterIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMOtherCounterIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if otherFastPathHits.Load() == 0 {
		b.Fatal("other fast path was not entered")
	}
}

func BenchmarkSSMConditionalFailureIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMConditionalFailureIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		if forceSSMFast {
			b.Fatal("fast path was not entered")
		}
	}
}

func BenchmarkSSMNestedRouteAssertionIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMNestedRouteAssertionIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if forceSSMFast {
		if ssmFastPathHits.Load() == 0 {
			b.Fatal("fast path was not entered")
		}
	}
}

func assertSSMFastPathProfile(*testing.B) {}

func BenchmarkSSMProfileAssertion(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMProfileAssertion reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	assertSSMFastPathProfile(b)
}

func assertEqual(*testing.B, int, int) {}

func BenchmarkSSMWrongGenericAssertionPolarity(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMWrongGenericAssertionPolarity reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	assertEqual(b, ssmFastPathHits.Load(), 0)
}

func BenchmarkSSMConditionalProfileAssertionIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMConditionalProfileAssertionIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if forceSSMFast {
		assertSSMFastPathProfile(b)
	}
}

func BenchmarkSSMEarlyReturnBypassesRouteAssertion(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMEarlyReturnBypassesRouteAssertion reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if forceSSMFast {
		return
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMGotoBypassesRouteAssertion(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMGotoBypassesRouteAssertion reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	goto done
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
done:
}

func BenchmarkSSMForwardGotoToRouteAssertion(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	goto check
check:
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMForwardGotoOverDeadReturnToRouteAssertion(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	goto check
	return
check:
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMForwardGotoOverDeadPanicToRouteAssertion(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	goto check
	panic("unreachable")
check:
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMParenthesizedPanicBypassesRouteAssertion(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMParenthesizedPanicBypassesRouteAssertion reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if forceSSMFast {
		(panic("skip assertion"))
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMParenthesizedPanicInitBypassesRouteAssertion(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMParenthesizedPanicInitBypassesRouteAssertion reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if (panic("skip assertion")); forceSSMFast {
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMParenthesizedPanicInRouteAssertionInitIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMParenthesizedPanicInRouteAssertionInitIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if (panic("skip assertion")); ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMRouteAssertionInitializerCounterWriteIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMRouteAssertionInitializerCounterWriteIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Store(1); ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMFailureBodyBypassIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMFailureBodyBypassIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		if forceSSMFast {
			return
		}
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMAllMissingCounterPathsFail(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	ssmFastPathHits.Store(0)
	for b.Loop() {
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		if forceSSMFast {
			b.Error("fast path was not entered")
		} else {
			b.Fatal("fast path was not entered")
		}
	}
}

func BenchmarkSSMBypassedResetIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	goto workload
	ssmFastPathHits.Store(0)
workload:
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMBypassedResetIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkSSMShadowedResetIsNotProof(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	{
		var ssmFastPathHits routeCounter
		ssmFastPathHits.Store(0)
	}
	for b.Loop() {
		// want +1 `benchmark BenchmarkSSMShadowedResetIsNotProof reaches ssm.*fixture does not prove guarded fast-path condition`
		ssm(delta, decay)
	}
	if ssmFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

func BenchmarkExpSubstringCounterIsNotProof(b *testing.B) {
	values := randomF64Tensor()
	unexpectedFastPathHits.Store(0)
	for b.Loop() {
		// want +1 `benchmark BenchmarkExpSubstringCounterIsNotProof reaches exp.*fixture does not prove guarded fast-path condition`
		exp(values)
	}
	if unexpectedFastPathHits.Load() == 0 {
		b.Fatal("fast path was not entered")
	}
}

//perfscan:benchmark-fast-path-validated external profile proves the route.
func BenchmarkSSMValidated(b *testing.B) {
	delta := randomF64Tensor()
	decay := randomF64Tensor()
	for b.Loop() {
		ssm(delta, decay)
	}
}

func BenchmarkStrictExpPositive(b *testing.B) {
	values := positiveMambaDelta()
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpReconstraintKeepsAliasLink(b *testing.B) {
	values := make([]float64, 2)
	alias := values
	fillPositive(values, 1)
	touch(alias)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpReconstraintKeepsAliasLink reaches strictExp.*requires > 0.*call to touch may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpRebindBreaksAliasLink(b *testing.B) {
	values := make([]float64, 2)
	alias := values
	values = positiveMambaDelta()
	touch(alias)
	for b.Loop() {
		strictExp(values)
	}
}

func BenchmarkStrictExpAggregateAliasAfterReconstraint(b *testing.B) {
	values := make([]float64, 2)
	holder := struct{ value []float64 }{value: values}
	fillPositive(values, 1)
	touch(holder.value)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAggregateAliasAfterReconstraint reaches strictExp.*requires > 0.*call to touch may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAggregateMemberAliasAfterReconstraint(b *testing.B) {
	values := make([]float64, 2)
	holder := struct{ value []float64 }{}
	holder.value = values
	fillPositive(values, 1)
	touch(holder.value)
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAggregateMemberAliasAfterReconstraint reaches strictExp.*requires > 0.*call to touch may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpAggregateCallArgumentMayMutate(b *testing.B) {
	values := make([]float64, 2)
	fillPositive(values, 1)
	mutateAggregateHolder(struct{ value []float64 }{value: values})
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAggregateCallArgumentMayMutate reaches strictExp.*requires > 0.*call to mutateAggregateHolder may mutate fixture is of unknown sign`
		strictExp(values)
	}
}

func BenchmarkStrictExpRoutedCallMutationCarries(b *testing.B) {
	values := positiveMambaDelta()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRoutedCallMutationCarries reaches mutatingStrictExp.*requires > 0.*loop-carried routed call may mutate fixture is of unknown sign`
		mutatingStrictExp(values)
	}
}

func BenchmarkStrictExpReadOnlyAliasDoesNotMutate(b *testing.B) {
	values := positiveMambaDelta()
	for b.Loop() {
		readOnlyAliasStrictExp(values)
	}
}

func BenchmarkStrictExpConditionalAliasMutationCarries(b *testing.B) {
	values := positiveMambaDelta()
	other := positiveMambaDelta()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpConditionalAliasMutationCarries reaches conditionalAliasMutatingStrictExp.*requires > 0.*loop-carried routed call may mutate fixture is of unknown sign`
		conditionalAliasMutatingStrictExp(values, other, forceSSMFast)
	}
}

func BenchmarkStrictExpLoopAliasMutationCarries(b *testing.B) {
	values := positiveMambaDelta()
	other := positiveMambaDelta()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpLoopAliasMutationCarries reaches loopAliasMutatingStrictExp.*requires > 0.*loop-carried routed call may mutate fixture is of unknown sign`
		loopAliasMutatingStrictExp(values, other, forceSSMFast)
	}
}

func BenchmarkStrictExpAggregateAliasMutationCarries(b *testing.B) {
	values := positiveMambaDelta()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAggregateAliasMutationCarries reaches aggregateAliasMutatingStrictExp.*requires > 0.*loop-carried routed call may mutate fixture is of unknown sign`
		aggregateAliasMutatingStrictExp(values)
	}
}

func BenchmarkStrictExpAggregateCallMutationCarries(b *testing.B) {
	values := positiveMambaDelta()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpAggregateCallMutationCarries reaches aggregateCallMutatingStrictExp.*requires > 0.*loop-carried routed call may mutate fixture is of unknown sign`
		aggregateCallMutatingStrictExp(values)
	}
}

func BenchmarkStrictExpUnconditionalIfInitRebindDoesNotCarry(b *testing.B) {
	values := positiveMambaDelta()
	other := positiveMambaDelta()
	for b.Loop() {
		unconditionalIfInitRebindStrictExp(values, other, forceSSMFast)
	}
}

func BenchmarkStrictExpRangeAliasMutationCarries(b *testing.B) {
	values := positiveRows()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpRangeAliasMutationCarries reaches rangeMutatingStrictExp.*requires > 0.*loop-carried routed call may mutate fixture is of unknown sign`
		rangeMutatingStrictExp(values)
	}
}

func BenchmarkStrictExpConversionMutationCarries(b *testing.B) {
	values := positiveMambaDelta()
	for b.Loop() {
		// want +1 `benchmark BenchmarkStrictExpConversionMutationCarries reaches conversionMutatingStrictExp.*requires > 0.*loop-carried routed call may mutate fixture is of unknown sign`
		conversionMutatingStrictExp(values)
	}
}

func BenchmarkNameOnlyNotExact(b *testing.T) {
	_ = b
}

func ordinarySemanticGuard(delta, decay []float64) {
	if allNonNegative(delta) && allNonPositive(decay) {
		expScalar(delta)
	} else {
		expScalar(decay)
	}
}
