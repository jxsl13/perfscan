package ps6101round15

import (
	"math"
	"math/rand"
	"testing"
)

var sink float64

func BenchmarkNestedForTransferBudget(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 256; i++ {
		for j := 0; j < 256; j++ {
			for k := 0; k < 256; k++ {
				for l := 0; l < 256; l++ {
					total := weights[0]
					if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
						sink = total
					}
				}
			}
		}
	}
}

func BenchmarkNestedForLateGateAfterBudget(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 256; i++ {
		for j := 0; j < 256; j++ {
			sink += float64(j & 1)
		}
		if i == 255 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
			weights[0] = 1
		}
	}
}

func BenchmarkForFirstRemainderIndexGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 257; i++ {
		if i == 256 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkForLaterReadBeforeWriteGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
			weights[0] = 1
		}
	}
}

func BenchmarkForImpossibleRemainderIndexSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 257; i++ {
		if i == 999 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkForRemainderBreakPreventsLateGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 256 {
			break
		}
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkForRemainderContinueSkipsSameIndexGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 256 {
			continue
		}
		if i == 256 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkForAbstractTailBreakPreventsLateGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 258 {
			break
		}
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkForAbstractTailContinueSkipsSameIndexGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 258 {
			continue
		}
		if i == 258 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkForAbstractTailGateBeforeBreak(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 258 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
			break
		}
	}
}

func BenchmarkForAbstractTailGateBeforeLaterBreak(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 258 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
		if i == 299 {
			break
		}
	}
}

func effectfulCase(calls *int, weights []float64) int {
	*calls++
	if *calls == 259 {
		weights[0] = 1
	}
	return 257
}

func BenchmarkEffectfulAbstractCaseEvaluatedOnce(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	calls := 0
	for i := 0; i < 300; i++ {
		if i == effectfulCase(&calls, weights) {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func effectfulBound(calls *int, weights []float64) int {
	*calls++
	if *calls == 513 {
		weights[0] = 1
	}
	return 300
}

func BenchmarkEffectfulForBoundEvaluatedOncePerCondition(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	calls := 0
	for i := 0; i < effectfulBound(&calls, weights); i++ {
		if i == 256 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkRangeImpossibleRemainderIndexSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 999 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkRangeLastRemainderIndexGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkIntegerRangeImpossibleRemainderIndexSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range 300 {
		if i == 999 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkIntegerRangeLastRemainderIndexGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range 300 {
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkRangeRemainderBreakPreventsLateGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 257 {
			break
		}
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkRangeRemainderContinueSkipsSameIndexGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 257 {
			continue
		}
		if i == 257 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkRangeAbstractTailBreakPreventsLateGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 258 {
			break
		}
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkRangeAbstractTailContinueSkipsSameIndexGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 258 {
			continue
		}
		if i == 258 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkRangeAbstractTailGateBeforeBreak(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 258 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
			break
		}
	}
}

func BenchmarkRangeAbstractTailGateBeforeLaterBreak(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 258 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
		if i == 299 {
			break
		}
	}
}

func BenchmarkForLaterLexicalGateStoppedByEarlierIndexBreak(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
		if i == 258 {
			break
		}
	}
}

func BenchmarkForLaterLexicalGateStoppedByEarlierIndexReturn(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
		if i == 258 {
			return
		}
	}
}

func BenchmarkForLaterLexicalGateStoppedByEarlierIndexGoto(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
		if i == 258 {
			goto done
		}
	}
done:
	sink += 0
}

func BenchmarkForOptionalEarlierBreakRetainsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := b.N < 0
	for i := 0; i < 300; i++ {
		if i == 258 && flag {
			break
		}
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkForOptionalEarlierReturnRetainsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := b.N < 0
	for i := 0; i < 300; i++ {
		if i == 258 && flag {
			return
		}
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkForStoredConvertedBoundRetainsLateGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	limit := int32(300)
	for i := 0; i < int(limit); i++ {
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkRangeLaterLexicalGateStoppedByEarlierIndexBreak(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
		if i == 258 {
			break
		}
	}
}

func BenchmarkRangeLaterLexicalGateStoppedByEarlierIndexReturn(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
		if i == 258 {
			return
		}
	}
}

func BenchmarkRangeLaterLexicalGateStoppedByEarlierIndexGoto(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
		if i == 258 {
			goto done
		}
	}
done:
	sink += 0
}

func BenchmarkRangeOptionalEarlierBreakRetainsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := b.N < 0
	for i := range [300]struct{}{} {
		if i == 258 && flag {
			break
		}
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkRangeOptionalEarlierReturnRetainsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := b.N < 0
	for i := range [300]struct{}{} {
		if i == 258 && flag {
			return
		}
		if i == 299 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkForEarlierIndexWriteSuppressesLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i == 258 {
			weights[0] = 1
		}
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkRangeEarlierIndexWriteSuppressesLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := range [300]struct{}{} {
		if i == 258 {
			weights[0] = 1
		}
		if i == 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	}
}

func BenchmarkForInequalityBreakStopsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 0; i < 300; i++ {
		if i >= 299 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
		if i >= 258 {
			break
		}
	}
}

func BenchmarkForTaggedSwitchBreakStopsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
loop:
	for i := 0; i < 300; i++ {
		switch i {
		case 299:
			total := weights[0]
			if total > 0 {
				sink = total
			}
		case 258:
			break loop
		}
	}
}

func BenchmarkForExpressionlessSwitchBreakStopsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
loop:
	for i := 0; i < 300; i++ {
		switch {
		case i == 299:
			total := weights[0]
			if total > 0 {
				sink = total
			}
		case i == 258:
			break loop
		}
	}
}

func BenchmarkForBooleanTaggedSwitchBreakStopsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
loop:
	for i := 0; i < 300; i++ {
		switch true {
		case i == 299:
			total := weights[0]
			if total > 0 {
				sink = total
			}
		case i == 258:
			break loop
		}
	}
}

func BenchmarkDescendingForBreakStopsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for i := 299; i >= 0; i-- {
		if i == 0 {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
		if i == 41 {
			break
		}
	}
}

func BenchmarkDescendingForOptionalBreakRetainsLaterGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	flag := b.N < 0
	for i := 299; i >= 0; i-- {
		if i == 41 && flag {
			break
		}
		if i == 0 {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func overwriteBLoopReceiver(b *testing.B, weights []float64) *testing.B {
	weights[0] = 1
	return b
}

func randomizeBLoopReceiver(b *testing.B, weights []float64) *testing.B {
	weights[0] = rand.NormFloat64()
	return b
}

func overwriteThenRandomizeBLoopReceiver(b *testing.B, weights []float64, calls *int) *testing.B {
	*calls++
	weights[0] = 1
	if *calls == 2 {
		weights[0] = rand.NormFloat64()
	}
	return b
}

func randomizeThenOverwriteBLoopReceiver(b *testing.B, weights []float64, calls *int) *testing.B {
	*calls++
	weights[0] = rand.NormFloat64()
	if *calls == 2 {
		weights[0] = 1
	}
	return b
}

func BenchmarkEffectfulBLoopReceiverOverwriteSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for overwriteBLoopReceiver(b, weights).Loop() {
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkEffectfulBLoopReceiverIntroducesGate(b *testing.B) {
	weights := []float64{1}
	for randomizeBLoopReceiver(b, weights).Loop() {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkEffectfulBLoopMethodExpressionOverwriteSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for (*testing.B).Loop(overwriteBLoopReceiver(b, weights)) {
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkEffectfulBLoopMethodExpressionIntroducesGate(b *testing.B) {
	weights := []float64{1}
	for (*testing.B).Loop(randomizeBLoopReceiver(b, weights)) {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkBLoopMethodExpressionReceiverEvaluatedOnceSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	calls := 0
	for (*testing.B).Loop(overwriteThenRandomizeBLoopReceiver(b, weights, &calls)) {
		total := weights[0]
		if total > 0 {
			sink = total
		}
		break
	}
}

func BenchmarkBLoopMethodExpressionReceiverEvaluatedOnceGate(b *testing.B) {
	weights := []float64{1}
	calls := 0
	for (*testing.B).Loop(randomizeThenOverwriteBLoopReceiver(b, weights, &calls)) {
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
		break
	}
}

func overwriteSwitchCase(weights []float64) int {
	weights[0] = 1
	return 0
}

func randomizeSwitchCase(weights []float64) int {
	weights[0] = rand.NormFloat64()
	return 0
}

func overwriteBooleanSwitchCase(weights []float64) bool {
	weights[0] = 1
	return true
}

func randomizeBooleanSwitchCase(weights []float64) bool {
	weights[0] = rand.NormFloat64()
	return true
}

func overwriteThenRandomizeSwitchCase(weights []float64, calls *int) bool {
	*calls++
	weights[0] = 1
	if *calls == 2 {
		weights[0] = rand.NormFloat64()
	}
	return true
}

func randomizeThenOverwriteSwitchCase(weights []float64, calls *int) bool {
	*calls++
	weights[0] = rand.NormFloat64()
	if *calls == 2 {
		weights[0] = 1
	}
	return true
}

func BenchmarkEffectfulSwitchCaseOverwriteSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	switch 0 {
	case overwriteSwitchCase(weights):
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkEffectfulSwitchCaseIntroducesGate(b *testing.B) {
	weights := []float64{1}
	switch 0 {
	case randomizeSwitchCase(weights):
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkEffectfulBooleanSwitchCaseOverwriteSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	switch true {
	case overwriteBooleanSwitchCase(weights):
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkEffectfulBooleanSwitchCaseIntroducesGate(b *testing.B) {
	weights := []float64{1}
	switch true {
	case randomizeBooleanSwitchCase(weights):
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkLogicalSwitchCaseEvaluatedOnceSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	calls := 0
	switch {
	case overwriteThenRandomizeSwitchCase(weights, &calls) && len(weights) == 1:
		total := weights[0]
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkLogicalSwitchCaseEvaluatedOnceGate(b *testing.B) {
	weights := []float64{1}
	calls := 0
	switch {
	case randomizeThenOverwriteSwitchCase(weights, &calls) && len(weights) == 1:
		total := weights[0]
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}

func BenchmarkRunParallelGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	})
}

func BenchmarkStoredRunParallelAndNextGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	run := b.RunParallel
	run(func(pb *testing.PB) {
		next := pb.Next
		for next() {
			total := weights[0]
			if total != 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	})
}

func BenchmarkRunParallelPositiveFixtureSilent(b *testing.B) {
	weights := []float64{math.Abs(rand.NormFloat64())}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	})
}

func BenchmarkStoppedRunParallelSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	b.StopTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	})
}

func BenchmarkStoppedStoredRunParallelSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	b.StopTimer()
	run := b.RunParallel
	run(func(pb *testing.PB) {
		next := pb.Next
		for next() {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	})
}

func BenchmarkStoppedMethodExpressionRunParallelSilent(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	b.StopTimer()
	run := (*testing.B).RunParallel
	next := (*testing.PB).Next
	run(b, func(pb *testing.PB) {
		for next(pb) {
			total := weights[0]
			if total > 0 {
				sink = total
			}
		}
	})
}

func BenchmarkTimedMethodExpressionRunParallelGate(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	run := (*testing.B).RunParallel
	next := (*testing.PB).Next
	run(b, func(pb *testing.PB) {
		for next(pb) {
			total := weights[0]
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	})
}
