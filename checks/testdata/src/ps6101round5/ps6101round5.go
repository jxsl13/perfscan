package ps6101round5

import (
	"math/rand"
	"testing"
)

var sink float64

var lowThreshold = -1e300
var highThreshold = 1e300

func overwriteHighThreshold() { highThreshold = 0 }

const typedPositiveDivisor float64 = 2

type namedFloat float64

func BenchmarkBaselineControlShouldReport(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	for b.Loop() {
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink += total
		}
	}
}

func BenchmarkPositiveConstantDivisionShouldReport(b *testing.B) {
	weight := rand.NormFloat64()
	denom := weight / 2
	for b.Loop() {
		if denom > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink += denom
		}
	}
}

func BenchmarkDistinctPackageThresholdProofMustNotSuppress(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	if total <= lowThreshold {
		b.Fatal("does not prove the high-threshold branch")
	}
	for b.Loop() {
		if total > highThreshold { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink += total
		}
	}
}

func BenchmarkExactCancellationMustNotReport(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight - weight
	for b.Loop() {
		if total > 0 {
			sink += total
		}
	}
}

func BenchmarkNestedExactRangesExposeAnalyzerScaling(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	for b.Loop() {
		for range 12 {
			for range 12 {
				for range 12 {
					if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
						sink += total
					}
				}
			}
		}
	}
}

func BenchmarkDeepNestedExactRangesRemainBounded(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	for range 24 {
		for range 24 {
			for range 24 {
				for range 24 {
					for range 24 {
						for range 24 {
							for range 24 {
								for range 24 {
									if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
										sink = total
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

func BenchmarkTypedPositiveDivisionShouldReport(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight / typedPositiveDivisor
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkNegativeConstantDivisionShouldReport(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight / -2
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkNamedConversionDivisionShouldReport(b *testing.B) {
	weight := rand.NormFloat64()
	total := float64(namedFloat(weight) / namedFloat(2))
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkNonconstantDivisionIsConservative(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight / rand.NormFloat64()
	if total > 0 {
		sink = total
	}
}

func BenchmarkSelfDivisionIsNotSymmetric(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight / weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkSamePackageThresholdProofSuppresses(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	if total <= highThreshold {
		b.Fatal("proves the same threshold")
	}
	if total > highThreshold {
		sink = total
	}
}

func BenchmarkAliasedSamePackageThresholdProofSuppresses(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	limit := highThreshold
	if total <= limit {
		b.Fatal("alias proves the same package threshold revision")
	}
	if total > highThreshold {
		sink = total
	}
}

func BenchmarkMutatedPackageThresholdInvalidatesProof(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	if total <= highThreshold {
		b.Fatal("proof applies only to the old threshold revision")
	}
	overwriteHighThreshold()
	if total > highThreshold { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkParenthesizedAliasCancellationIsZero(b *testing.B) {
	weight := rand.NormFloat64()
	alias := weight
	total := (weight) - (alias)
	if total > 0 {
		sink = total
	}
}

func BenchmarkNamedCancellationIsZero(b *testing.B) {
	weight := namedFloat(rand.NormFloat64())
	total := weight - weight
	if total > 0 {
		sink = float64(total)
	}
}

func BenchmarkPossiblyOverflowedCancellationIsConservative(b *testing.B) {
	weight := rand.NormFloat64()
	scaled := weight * 1e308
	total := scaled - scaled
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkOppositeSignSubtractionShouldReport(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight - -weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkDistinctInputsSubtractionShouldReport(b *testing.B) {
	weight := rand.NormFloat64()
	otherWeight := rand.NormFloat64()
	total := weight - otherWeight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
		sink = total
	}
}

func BenchmarkLateRangeGateIsVisitedAbstractly(b *testing.B) {
	weight := rand.NormFloat64()
	total := weight
	for index := range 1024 {
		if index == 1000 {
			if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
				sink = total
			}
		}
	}
}

func BenchmarkSmallExactNestedOverwriteRemainsExact(b *testing.B) {
	weight := rand.NormFloat64()
	for range 2 {
		for range 2 {
			weight = 1
		}
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkSecondExactIterationOverwriteRemainsExact(b *testing.B) {
	weight := rand.NormFloat64()
	for index := range 2 {
		if index == 1 {
			weight = 1
		}
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkBudgetExhaustedSingleTripRemainsExact(b *testing.B) {
	for range 256 {
	}
	weight := rand.NormFloat64()
	for index := range 1 {
		if index == 0 {
			weight = 1
		}
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkBudgetExhaustedTwoTripsRemainExact(b *testing.B) {
	for range 256 {
	}
	weight := rand.NormFloat64()
	for index := range 2 {
		if index == 1 {
			weight = 1
		}
	}
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkBudgetExhaustedFirstBreakRemainsExact(b *testing.B) {
	for range 256 {
	}
	weight := rand.NormFloat64()
	total := weight
outer:
	for index := range 1024 {
		if index == 0 {
			break outer
		}
		if total > 0 {
			sink = total
		}
	}
}

func BenchmarkBudgetExhaustedLateContinueGateIsVisited(b *testing.B) {
	for range 256 {
	}
	weight := rand.NormFloat64()
	total := weight
	for index := range 1024 {
		if index < 1000 {
			continue
		}
		if total > 0 { // want `benchmark feeds symmetric signed random inputs into a sign/nonzero/threshold gate`
			sink = total
		}
	}
}
