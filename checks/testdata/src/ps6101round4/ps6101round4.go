package ps6101round4

import (
	"math/rand"
	"runtime"
	"testing"
	"unsafe"
)

var sink float64
var skipPreparation = true

func overwriteFalse(weight *float64) bool { *weight = 1; return false }
func overwriteTrue(weight *float64) bool  { *weight = 1; return true }

func BenchmarkUnknownAndShortCircuit(b *testing.B) {
	weight := rand.NormFloat64()
	if rand.Intn(2) == 0 && overwriteFalse(&weight) {
		sink = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkUnknownOrShortCircuit(b *testing.B) {
	weight := rand.NormFloat64()
	if rand.Intn(2) == 0 || overwriteTrue(&weight) {
		sink = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkKnownAndExecutesOverwrite(b *testing.B) {
	weight := rand.NormFloat64()
	if true && overwriteFalse(&weight) {
		sink = 1
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkKnownOrSkipsOverwrite(b *testing.B) {
	weight := rand.NormFloat64()
	if true || overwriteTrue(&weight) {
		sink = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkConditionalContinueRetainsInput(b *testing.B) {
	weight := rand.NormFloat64()
	for range [1]int{} {
		if rand.Intn(2) == 0 {
			continue
		}
		weight = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkConditionalBreakRetainsInput(b *testing.B) {
	weight := rand.NormFloat64()
	for range [1]int{} {
		if rand.Intn(2) == 0 {
			break
		}
		weight = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkNestedSwitchBreakDoesNotExitRange(b *testing.B) {
	weight := rand.NormFloat64()
	for range [1]int{} {
		switch rand.Intn(2) {
		case 0:
			break
		default:
		}
		weight = 1
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkNestedSelectBreakDoesNotExitRange(b *testing.B) {
	weight := rand.NormFloat64()
	for range [1]int{} {
		select {
		default:
			break
		}
		weight = 1
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkContinueInsideSwitchTargetsRange(b *testing.B) {
	weight := rand.NormFloat64()
	iteration := 0
	for range [2]int{} {
		iteration++
		switch {
		case iteration < 2:
			continue
		}
		weight = 1
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkMapCapacityIsNotLength(b *testing.B) {
	weight := rand.NormFloat64()
	values := make(map[int]int, 1)
	for range values {
		weight = 1
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkMapRangeShrinksDuringIteration(b *testing.B) {
	weight := rand.NormFloat64()
	values := map[int]int{0: 0, 1: 0}
	iteration := 0
	for range values {
		if iteration > 0 {
			weight = 1
		}
		clear(values)
		iteration++
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkImmutableMapLiteralHasTwoTrips(b *testing.B) {
	weight := rand.NormFloat64()
	values := map[int]int{0: 0, 1: 0}
	iteration := 0
	for range values {
		if iteration > 0 {
			weight = 1
		}
		iteration++
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkMapDeletesAllRemainingEntries(b *testing.B) {
	weight := rand.NormFloat64()
	values := map[int]int{0: 0, 1: 0}
	iteration := 0
	for range values {
		if iteration > 0 {
			weight = 1
		}
		delete(values, 0)
		delete(values, 1)
		iteration++
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkCapturedClosureClearsMapRange(b *testing.B) {
	weight := rand.NormFloat64()
	values := map[int]int{0: 0, 1: 0}
	clearValues := func() { clear(values) }
	iteration := 0
	for range values {
		if iteration > 0 {
			weight = 1
		}
		clearValues()
		iteration++
	}
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkMapInsertionCannotRemoveOriginalTrips(b *testing.B) {
	weight := rand.NormFloat64()
	values := map[int]int{0: 0, 1: 0}
	iteration := 0
	for range values {
		if iteration == 0 {
			values[2] = 0
		}
		if iteration > 0 {
			weight = 1
		}
		iteration++
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkInsertedMapIsNotStillKnownEmpty(b *testing.B) {
	weight := rand.NormFloat64()
	values := make(map[int]int, 4)
	values[0] = 0
	for range values {
		weight = 1
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

type embeddedWeights struct{ Weights []float64 }
type promotedWeights struct{ embeddedWeights }
type pointerPromotedWeights struct{ *promotedWeights }

func BenchmarkPromotedSliceFieldOverwrite(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	box := promotedWeights{embeddedWeights: embeddedWeights{Weights: weights}}
	box.Weights[0] = 1
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkMultiLevelPointerPromotedOverwrite(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	box := pointerPromotedWeights{promotedWeights: &promotedWeights{embeddedWeights{Weights: weights}}}
	box.Weights[0] = 1
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

type namedWeights []float64
type namedMap map[string]float64
type convertedAggregate promotedWeights

func overwriteNamedWeights(weights namedWeights) { weights[0] = 1 }
func overwriteNamedMap(weights namedMap)         { weights["weight"] = 1 }
func overwriteConvertedAggregate(weights convertedAggregate) {
	weights.Weights[0] = 1
}

func BenchmarkNamedSliceConversionOverwrite(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	overwriteNamedWeights(namedWeights(weights))
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkNamedMapConversionOverwrite(b *testing.B) {
	weights := map[string]float64{"weight": rand.NormFloat64()}
	overwriteNamedMap(namedMap(weights))
	total := weights["weight"]
	if total > 0 {
		sink = total
	}
}

func BenchmarkAggregateConversionPreservesSliceDescendant(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	box := promotedWeights{embeddedWeights{Weights: weights}}
	overwriteConvertedAggregate(convertedAggregate(box))
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

type valueOnly struct{ Weight float64 }
type convertedValueOnly valueOnly

func overwriteConvertedCopy(value convertedValueOnly) { value.Weight = 1 }

func BenchmarkValueOnlyConversionStaysIndependent(b *testing.B) {
	value := valueOnly{Weight: rand.NormFloat64()}
	overwriteConvertedCopy(convertedValueOnly(value))
	total := value.Weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkUnsafePointerOverwriteIsConservative(b *testing.B) {
	weight := rand.NormFloat64()
	pointer := (*float64)(unsafe.Pointer(&weight))
	*pointer = 1
	total := weight
	if total > 0 {
		sink = total
	}
}

func wrapWeights(weights []float64) promotedWeights {
	return promotedWeights{embeddedWeights: embeddedWeights{Weights: weights}}
}

func wrapWeightsPair(weights []float64) (promotedWeights, bool) {
	return wrapWeights(weights), true
}

func forwardWeights(weights []float64) promotedWeights { return wrapWeights(weights) }

type returnedInner struct{ Values []float64 }
type returnedOuter struct{ Items [1]returnedInner }

func wrapNestedWeights(weights []float64) returnedOuter {
	return returnedOuter{Items: [1]returnedInner{{Values: weights}}}
}

func BenchmarkReturnedAggregateOverwrite(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	box := wrapWeights(weights)
	box.Weights[0] = 1
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkCrossHelperReturnedAggregateOverwrite(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	box := forwardWeights(weights)
	box.Weights[0] = 1
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkMultipleReturnAggregateOverwrite(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	box, ok := wrapWeightsPair(weights)
	if !ok {
		b.Fatal("missing box")
	}
	box.Weights[0] = 1
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkNestedReturnedAggregateOverwrite(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	box := wrapNestedWeights(weights)
	box.Items[0].Values[0] = 1
	total := weights[0]
	if total > 0 {
		sink = total
	}
}

func BenchmarkSubbenchmarkEarlyReturnRetainsInput(b *testing.B) {
	weight := rand.NormFloat64()
	b.Run("prepare", func(b *testing.B) {
		if skipPreparation {
			return
		}
		weight = 1
	})
	b.Run("measure", func(b *testing.B) {
		total := weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	})
}

func BenchmarkStoredSubbenchmarkClosureOverwrites(b *testing.B) {
	weight := rand.NormFloat64()
	prepare := func(b *testing.B) { weight = 1 }
	b.Run("prepare", prepare)
	b.Run("measure", func(b *testing.B) {
		total := weight
		if total > 0 {
			sink = total
		}
	})
}

func BenchmarkStoredSubbenchmarkClosureEarlyReturn(b *testing.B) {
	weight := rand.NormFloat64()
	prepare := func(b *testing.B) {
		if skipPreparation {
			return
		}
		weight = 1
	}
	b.Run("prepare", prepare)
	b.Run("measure", func(b *testing.B) {
		total := weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	})
}

func BenchmarkPanickingSubbenchmarkDoesNotReturnRisk(b *testing.B) {
	weight := rand.NormFloat64()
	b.Run("prepare", func(b *testing.B) {
		if skipPreparation {
			panic("preparation failed")
		}
		weight = 1
	})
	b.Run("measure", func(b *testing.B) {
		total := weight
		if total > 0 {
			sink = total
		}
	})
}

func BenchmarkGoexitSubbenchmarkDoesNotFallThrough(b *testing.B) {
	weight := rand.NormFloat64()
	b.Run("prepare", func(b *testing.B) {
		if skipPreparation {
			runtime.Goexit()
		}
		weight = 1
	})
	b.Run("measure", func(b *testing.B) {
		total := weight
		if total > 0 {
			sink = total
		}
	})
}
