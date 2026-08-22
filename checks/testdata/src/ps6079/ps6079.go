package ps6079

import (
	"math"
	"sync/atomic"
	"testing"

	"ps6079dep"
)

var forceSSMFast = true
var strictExpGlobalValues = []float64{1}

func mutateStrictExpGlobalValues() { strictExpGlobalValues[0] = -1 }
func skipBenchmark(b *testing.B)   { b.SkipNow() }

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

func randomF64Tensor() []float64               { return []float64{-1, 1} }
func makeRandomTensor() []float64              { return []float64{-1, 1} }
func unknownTensor() []float64                 { return []float64{-1, 1} }
func nonNegativeTensor() []float64             { return []float64{0, 1} }
func nonPositiveTensor() []float64             { return []float64{0, -1} }
func positiveMambaDelta() []float64            { return []float64{1, 2} }
func negativeMambaDecay() []float64            { return []float64{-1, -2} }
func positiveMap() map[int]float64             { return map[int]float64{1: 1} }
func fillRandom([]float64)                     {}
func fillPositive([]float64, float64)          {}
func touch([]float64)                          {}
func touchPointerMap(map[*[]float64]struct{})  {}
func mutateScalar(*float64)                    {}
func fillPositiveFrom([]float64, []float64)    {}
func fillPositiveWithScale(float64, []float64) {}
func identity(values []float64) []float64      { return values }
func pair(values []float64) ([]float64, bool)  { return values, true }
func invokeCallback(callback func())           { callback() }
func invokeAny(callback any)                   { callback.(func())() }
func mutateAny(value any)                      { value.([]float64)[0] = -1 }
func ssmNEON([]float64, []float64)             {}
func ssmScanDRangeScalar([]float64, []float64) {}

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
