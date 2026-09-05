package ps6101round3

import (
	"math/rand"
	"testing"
)

var sink float64

func randomWeight(seed int64) float64 {
	return rand.New(rand.NewSource(seed)).NormFloat64()
}

func BenchmarkDirectPointerFieldOverwrite(b *testing.B) {
	weight := randomWeight(1)
	box := struct{ Weight *float64 }{Weight: &weight}
	*box.Weight = 1
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendThroughZeroLengthAlias(b *testing.B) {
	weights := make([]float64, 1, 2)
	weights[0] = randomWeight(2)
	_ = append(weights[:0], 1)
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendAssignmentOverwritesInput(b *testing.B) {
	weights := []float64{randomWeight(58)}
	weights = append(weights[:0], 1)
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendAssignmentKeepsAppendedRisk(b *testing.B) {
	weights := []float64{}
	weights = append(weights, randomWeight(59))
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkAppendAssignmentTracksScalarElement(b *testing.B) {
	weights := []float64{randomWeight(60)}
	weights = append(weights, 1)
	total := weights[1]
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendAssignmentKeepsPrefixRisk(b *testing.B) {
	weights := []float64{randomWeight(61)}
	weights = append(weights, 1)
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkVacuousCounterThroughPointerField(b *testing.B) {
	weight := randomWeight(3)
	total := weight
	var hotBranches int
	box := struct{ Counter *int }{Counter: &hotBranches}
	*box.Counter = 1
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkSecondRangeIterationOverwritesInput(b *testing.B) {
	weights := []float64{randomWeight(4)}
	i := 0
	for range []int{0, 0} {
		if i > 0 {
			weights[0] = 1
		}
		i++
	}
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkVacuousCounterOnSecondRangeIteration(b *testing.B) {
	weight := randomWeight(5)
	total := weight
	var hotBranches int
	i := 0
	for range []int{0, 0} {
		if i > 0 {
			hotBranches = 1
		}
		i++
	}
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkForwardGotoSkipsPositiveProof(b *testing.B) {
	weight := randomWeight(6)
	total := weight
	goto timed
	if total <= 0 {
		b.Fatal("total must be positive")
	}
timed:
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkVacuousCounterThroughStoredClosure(b *testing.B) {
	weight := randomWeight(7)
	total := weight
	var hotBranches int
	mutate := func() { hotBranches = 1 }
	mutate()
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkStoredClosureOverwritesInput(b *testing.B) {
	weight := randomWeight(8)
	mutate := func() { weight = 1 }
	mutate()
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkImplicitZeroStructCounter(b *testing.B) {
	weight := randomWeight(9)
	total := weight
	var counters struct{ HotBranches int }
	if total > 0 {
		counters.HotBranches++
		sink = total
	}
	if counters.HotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func takeRandomThenOverwrite(weight *float64) float64 {
	total := *weight
	*weight = 1
	return total
}

func overwriteAndFalse(weight *float64) bool {
	*weight = 1
	return false
}

func overwriteIndex(weight *float64) int {
	*weight = 1
	return 0
}

func BenchmarkSideEffectingIfConditionEvaluatedOnce(b *testing.B) {
	weight := randomWeight(10)
	if takeRandomThenOverwrite(&weight) > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = weight
	}
}

func BenchmarkBareBooleanConditionExecutesEffects(b *testing.B) {
	weight := randomWeight(56)
	if overwriteAndFalse(&weight) {
		sink = weight
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkIndexedBooleanConditionExecutesEffects(b *testing.B) {
	weight := randomWeight(57)
	flags := []bool{false}
	if flags[overwriteIndex(&weight)] {
		sink = weight
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkNestedPointerFieldOverwrite(b *testing.B) {
	weight := randomWeight(11)
	var box struct{ Inner struct{ Weight *float64 } }
	box.Inner.Weight = &weight
	*box.Inner.Weight = 1
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkArrayPointerElementOverwrite(b *testing.B) {
	weight := randomWeight(12)
	references := [1]*float64{&weight}
	*references[0] = 1
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkMapPointerElementOverwrite(b *testing.B) {
	weight := randomWeight(13)
	references := map[string]*float64{"weight": &weight}
	*references["weight"] = 1
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkMapPointerDynamicConstantOverwrite(b *testing.B) {
	weight := randomWeight(14)
	references := map[string]*float64{"weight": &weight}
	key := "weight"
	*references[key] = 1
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkStructSliceElementOverwrite(b *testing.B) {
	weights := []float64{randomWeight(15)}
	box := struct{ Weights []float64 }{Weights: weights}
	box.Weights[0] = 1
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkStructMapElementOverwrite(b *testing.B) {
	weights := map[string]float64{"weight": randomWeight(16)}
	box := struct{ Weights map[string]float64 }{Weights: weights}
	box.Weights["weight"] = 1
	total := weights["weight"]
	if total > 0 {
		sink = total
	}
}

type safeValue struct{ Weight float64 }

func overwriteSafeCopy(value safeValue) { value.Weight = 1 }

func BenchmarkValueOnlyStructCopyRemainsIndependent(b *testing.B) {
	box := safeValue{Weight: randomWeight(17)}
	overwriteSafeCopy(box)
	total := box.Weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

type pointerMutator struct{ Weight *float64 }

func (mutator pointerMutator) Overwrite() { *mutator.Weight = 1 }

func BenchmarkStoredMethodValueOverwritesInput(b *testing.B) {
	weight := randomWeight(18)
	mutator := pointerMutator{Weight: &weight}
	overwrite := mutator.Overwrite
	overwrite()
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkStoredMethodValueFreezesReceiver(b *testing.B) {
	weight := randomWeight(63)
	other := 2.0
	mutator := pointerMutator{Weight: &weight}
	overwrite := mutator.Overwrite
	mutator.Weight = &other
	overwrite()
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkStoredMethodExpressionOverwritesInput(b *testing.B) {
	weight := randomWeight(55)
	mutator := pointerMutator{Weight: &weight}
	overwrite := pointerMutator.Overwrite
	overwrite(mutator)
	total := weight
	if total > 0 {
		sink = total
	}
}

type counterMutator struct{ Counter *int }

func (mutator counterMutator) Overwrite() { *mutator.Counter = 1 }

func BenchmarkStoredMethodValueInvalidatesCounter(b *testing.B) {
	weight := randomWeight(19)
	total := weight
	var hotBranches int
	mutator := counterMutator{Counter: &hotBranches}
	overwrite := mutator.Overwrite
	overwrite()
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches++
		sink = total
	}
	if hotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkClosureAliasOverwritesInput(b *testing.B) {
	weight := randomWeight(20)
	mutate := func() { weight = 1 }
	alias := mutate
	alias()
	total := weight
	if total > 0 {
		sink = total
	}
}

func recursiveOverwrite(weight *float64, depth int) {
	if depth == 0 {
		*weight = 1
		return
	}
	recursiveOverwrite(weight, depth-1)
}

func BenchmarkRecursiveHelperIsConservative(b *testing.B) {
	weight := randomWeight(62)
	recursiveOverwrite(&weight, 1)
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkStoredClosureWithoutRelevantMutation(b *testing.B) {
	weight := randomWeight(21)
	mutate := func() { sink = 1 }
	mutate()
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkClosureInStructOverwritesInput(b *testing.B) {
	weight := randomWeight(22)
	holder := struct{ Mutate func() }{Mutate: func() { weight = 1 }}
	holder.Mutate()
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendFullCapacityAllocates(b *testing.B) {
	weights := []float64{randomWeight(23)}
	full := weights[:len(weights):len(weights)]
	_ = append(full, 1)
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkAppendSpareCapacityPreservesEarlierElement(b *testing.B) {
	weights := make([]float64, 1, 2)
	weights[0] = randomWeight(24)
	_ = append(weights, 1)
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkAppendOffsetOverwritesOtherElement(b *testing.B) {
	weights := []float64{randomWeight(25), randomWeight(26)}
	_ = append(weights[1:1], 1)
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkAppendOffsetOverwritesReadElement(b *testing.B) {
	weights := []float64{1, randomWeight(27)}
	_ = append(weights[1:1], 1)
	total := weights[1]
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendStoredZeroLengthView(b *testing.B) {
	weights := []float64{randomWeight(28)}
	view := weights[:0]
	_ = append(view, 1)
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendEmptyEllipsisPreservesInput(b *testing.B) {
	weights := []float64{randomWeight(29)}
	empty := []float64{}
	_ = append(weights[:0], empty...)
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkAppendEllipsisOverwritesInput(b *testing.B) {
	weights := []float64{randomWeight(30)}
	ones := []float64{1}
	_ = append(weights[:0], ones...)
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkZeroIterationRangePreservesInput(b *testing.B) {
	weight := randomWeight(31)
	for range [0]int{} {
		weight = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkOneIterationRangeOverwritesInput(b *testing.B) {
	weight := randomWeight(32)
	for range [1]int{} {
		weight = 1
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkUnknownRangeWithoutWritesPreservesInput(b *testing.B) {
	weight := randomWeight(33)
	values := make([]int, b.N)
	for range values {
		sink = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkUnknownRangeMayOverwriteInput(b *testing.B) {
	weight := randomWeight(34)
	values := make([]int, b.N)
	for range values {
		weight = 1
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkRangeBreakSkipsOverwrite(b *testing.B) {
	weight := randomWeight(35)
	for range []int{0, 0} {
		break
		weight = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkRangeContinueReachesSecondOverwrite(b *testing.B) {
	weight := randomWeight(36)
	i := 0
	for range []int{0, 0} {
		i++
		if i < 2 {
			continue
		}
		weight = 1
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkLabeledRangeIsAnalyzed(b *testing.B) {
	weight := randomWeight(37)
outer:
	for range []int{0} {
		weight = 1
		break outer
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkLabeledBreakEscapesOuterRange(b *testing.B) {
	weight := randomWeight(38)
outer:
	for range []int{0, 0} {
		for range []int{0} {
			break outer
		}
		weight = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkLabeledContinueSkipsOuterOverwrite(b *testing.B) {
	weight := randomWeight(39)
outer:
	for range []int{0, 0} {
		for range []int{0} {
			continue outer
		}
		weight = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkFalseShortCircuitSkipsMutation(b *testing.B) {
	weight := randomWeight(40)
	mutate := func() bool { weight = 1; return true }
	if false && mutate() {
		sink = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkTrueShortCircuitSkipsMutation(b *testing.B) {
	weight := randomWeight(41)
	mutate := func() bool { weight = 1; return false }
	if true || mutate() {
		sink = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkGotoSkipsOverwrite(b *testing.B) {
	weight := randomWeight(42)
	goto timed
	weight = 1
timed:
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkConditionalGotoMergesWithFallthrough(b *testing.B) {
	weight := randomWeight(43)
	if b.N > 1 {
		goto timed
	}
	weight = 1
timed:
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkImplicitZeroArrayCounter(b *testing.B) {
	weight := randomWeight(44)
	total := weight
	var hotBranches [1]int
	if total > 0 {
		hotBranches[0]++
		sink = total
	}
	if hotBranches[0] == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkImplicitZeroArrayCounterRuntimeIndex(b *testing.B) {
	weight := randomWeight(64)
	total := weight
	var hotBranches [1]int
	index := 0
	if total > 0 {
		hotBranches[index]++
		sink = total
	}
	if hotBranches[index] == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkRuntimeIndexCounterMustStillStartAtZero(b *testing.B) {
	weight := randomWeight(65)
	total := weight
	var hotBranches [1]int
	index := 0
	hotBranches[index] = 1
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		hotBranches[index]++
		sink = total
	}
	if hotBranches[index] == 0 {
		b.Fatal("hot branch not entered")
	}
}

func BenchmarkImplicitZeroNestedCounter(b *testing.B) {
	weight := randomWeight(45)
	total := weight
	var counters struct{ Inner [1]struct{ HotBranches int } }
	if total > 0 {
		counters.Inner[0].HotBranches++
		sink = total
	}
	if counters.Inner[0].HotBranches == 0 {
		b.Fatal("hot branch not entered")
	}
}

func unknownReferenceKey(b *testing.B) string {
	if b.N > 1 {
		return "weight"
	}
	return "also-weight"
}

func BenchmarkDynamicMapPointerPointeeOverwrite(b *testing.B) {
	weight := randomWeight(46)
	references := map[string]*float64{"weight": &weight, "also-weight": &weight}
	*references[unknownReferenceKey(b)] = 1
	total := weight
	if total > 0 {
		sink = total
	}
}

type nestedReferences struct {
	Values [1]struct{ Weights []float64 }
}

func overwriteNestedReference(value nestedReferences) { value.Values[0].Weights[0] = 1 }

func BenchmarkNestedAggregateBackingOverwrite(b *testing.B) {
	weights := []float64{randomWeight(47)}
	value := nestedReferences{}
	value.Values[0].Weights = weights
	overwriteNestedReference(value)
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendOverlappingSourceOverwritesInput(b *testing.B) {
	weights := []float64{randomWeight(48), 1}
	_ = append(weights[:0], weights[1:]...)
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendStoredOffsetViewOverwritesInput(b *testing.B) {
	weights := []float64{1, randomWeight(53)}
	view := weights[1:1]
	_ = append(view, 1)
	total := weights[1]
	if total > 0 {
		sink = total
	}
}

func BenchmarkAppendStoredCappedViewAllocates(b *testing.B) {
	weights := []float64{randomWeight(54)}
	view := weights[:1:1]
	_ = append(view, 1)
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkThreeIterationRangeOverwritesInput(b *testing.B) {
	weight := randomWeight(49)
	iteration := 0
	for range [3]int{} {
		iteration++
		if iteration == 3 {
			weight = 1
		}
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkLabeledForBreakSkipsOuterOverwrite(b *testing.B) {
	weight := randomWeight(50)
outer:
	for iteration := 0; iteration < 2; iteration++ {
		for range [1]int{} {
			break outer
		}
		weight = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkLabeledForContinueSkipsOuterOverwrite(b *testing.B) {
	weight := randomWeight(51)
outer:
	for iteration := 0; iteration < 2; iteration++ {
		for range [1]int{} {
			continue outer
		}
		weight = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkBackwardGotoIsControlAmbiguous(b *testing.B) {
	weight := randomWeight(52)
	iteration := 0
loop:
	iteration++
	if iteration < b.N {
		goto loop
	}
	total := weight
	if total > 0 {
		sink = total
	}
}
