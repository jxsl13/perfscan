package ps6101round17

import (
	"math/rand"
	"testing"
)

var (
	sink   float64
	toggle bool
)

type loopFunc func() bool

func loopWithPreResetGate(b *testing.B, weights []float64) func() bool {
	total := weights[0]
	if total > 0 {
		sink = total
	}
	return b.Loop
}

func convertedLoopWithPreResetGate(b *testing.B, weights []float64) loopFunc {
	total := weights[0]
	if total > 0 {
		sink = total
	}
	return loopFunc(b.Loop)
}

func BenchmarkReturnedBLoopMethodPreResetGateIsUntimed(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for loopWithPreResetGate(b, weights)() {
		break
	}
}

func BenchmarkReturnedBLoopMethodBodyIsTimed(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for loopWithPreResetGate(b, weights)() {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
		break
	}
}

func BenchmarkStoredReturnedBLoopMethodBodyIsTimed(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	loop := loopWithPreResetGate(b, weights)
	for loop() {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
		break
	}
}

func BenchmarkConvertedReturnedBLoopMethodPreResetGateIsUntimed(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for convertedLoopWithPreResetGate(b, weights)() {
		break
	}
}

func BenchmarkConvertedReturnedBLoopMethodBodyIsTimed(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for convertedLoopWithPreResetGate(b, weights)() {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
		break
	}
}

func overwriteThenRandomizeReturnedLoop(b *testing.B, weights []float64, calls *int) func() bool {
	*calls++
	weights[0] = 1
	if *calls == 2 {
		weights[0] = rand.NormFloat64()
	}
	return b.Loop
}

func randomizeThenOverwriteReturnedLoop(b *testing.B, weights []float64, calls *int) func() bool {
	*calls++
	weights[0] = rand.NormFloat64()
	if *calls == 2 {
		weights[0] = 1
	}
	return b.Loop
}

func BenchmarkReturnedBLoopFunctionOperandEvaluatedOnceSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	calls := 0
	for overwriteThenRandomizeReturnedLoop(b, weights, &calls)() {
		total := weights[0]
		if total > 0 {
			sink = total
		}
		break
	}
}

func BenchmarkReturnedBLoopFunctionOperandEvaluatedOnceGate(b *testing.B) {
	weights := []float64{1}
	calls := 0
	for randomizeThenOverwriteReturnedLoop(b, weights, &calls)() {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
		break
	}
}

func BenchmarkBLoopBreakLeavesTimerRunning(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for b.Loop() {
		break
	}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkBLoopNormalCompletionStopsTimer(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for b.Loop() {
		sink += 1
	}
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkBLoopConditionalBreakMayLeaveTimerRunning(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for b.Loop() {
		if toggle {
			break
		}
	}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkBLoopLabeledBreakLeavesTimerRunning(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
loop:
	for b.Loop() {
		break loop
	}
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkBLoopGotoLeavesTimerRunning(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for b.Loop() {
		goto done
	}
done:
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func returnFromBLoopBody(b *testing.B) {
	for b.Loop() {
		return
	}
}

func BenchmarkBLoopReturnLeavesTimerRunning(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	returnFromBLoopBody(b)
	total := weights[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkMutatedStoredBoundStopsTail(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	limit := 300
	for i := 0; i < limit; i++ {
		if i == 257 {
			limit = 257
		}
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkMutatedInductionStopsTail(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
		if i == 257 {
			i = 299
		}
	}
}

func BenchmarkDescendingMutatedBoundStopsTail(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	limit := 0
	for i := 299; i >= limit; i-- {
		if i == 42 {
			limit = 42
		}
		if i == 0 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkConvertedStoredBoundStopsTail(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	limit := int64(300)
	for i := int64(0); i < int64(limit); i++ {
		if i == 257 {
			limit = int64(257)
		}
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkRaisedStoredBoundRetainsReachableGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	limit := 300
	for i := 0; i < limit; i++ {
		if i == 257 {
			limit = 301
		}
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkIntegerRangeCapturesOriginalBound(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	limit := 300
	for i := range limit {
		if i == 257 {
			limit = 257
		}
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkRangeRebindsMutatedInduction(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range 300 {
		if i == 257 {
			i = 999
		}
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func overwriteTrue(weights []float64) bool {
	weights[0] = 1
	return true
}

func randomizeTrue(weights []float64) bool {
	weights[0] = rand.NormFloat64()
	return true
}

func stopTimerTrue(b *testing.B) bool {
	b.StopTimer()
	return true
}

func overwriteAndMiss(weights []float64) int {
	weights[0] = 1
	return 1
}

func BenchmarkExpressionlessSelectedAndUsesExecutedRHSState(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := toggle
	switch {
	case flag && overwriteTrue(weights):
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkExpressionlessSelectedAndCanIntroduceRisk(b *testing.B) {
	weights := []float64{1}
	flag := toggle
	switch {
	case flag && randomizeTrue(weights):
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkTaggedSelectedAndIsUntimed(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := toggle
	switch true {
	case flag && stopTimerTrue(b):
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkTaggedSelectedOrMayRemainTimed(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := toggle
	switch true {
	case flag || stopTimerTrue(b):
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkExpressionlessAndNoMatchKeepsSkippedState(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := toggle
	switch {
	case flag && overwriteTrue(weights):
	default:
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkTaggedAndFallthroughKeepsExecutedTimerState(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := toggle
	switch true {
	case flag && stopTimerTrue(b):
		fallthrough
	case false:
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkEarlierNonmatchCaseOperandAffectsLaterMatch(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	switch 0 {
	case overwriteAndMiss(weights):
	case 0:
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkLogicalShortCircuitSkipsRHSOverwrite(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	switch {
	case true || overwriteTrue(weights):
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}
