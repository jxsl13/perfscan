package ps6011

import "testing"

func explicitCastVendorMatmul() {}
func graphFusedMatmul()         {}
func reportSpeedRatio()         {}

func BenchmarkGraphMatmulPromotion(b *testing.B) { // want `graph/compiler-fused matmul timing gate has no numerical-parity manifest; matching visible dtypes do not prove matching accumulator/reduction semantics; missing explicit-conversion control route`
	for range b.N {
		explicitCastVendorMatmul()
		graphFusedMatmul()
		reportSpeedRatio()
	}
}

func TestGraphMatmulPromotionGate(t *testing.T) { // want `graph/compiler-fused matmul timing gate has no numerical-parity manifest`
	explicitCastVendorMatmul()
	graphFusedMatmul()
	reportSpeedRatio()
}

type shape struct {
	M int
	K int
	N int
}

type graphMatmulParityGate struct {
	ExplicitConversionControlRoute string
	GraphFusedCandidateRoute       string
	SmallShape                     shape
	BroadProductionShape           shape
	SmallSameShapeComparator       string
	ProductionSameShapeComparator  string
	FiniteOutputGate               bool
	RelativeErrorTolerance         float64
}

func BenchmarkGraphMatmulPromotionPartial(b *testing.B) {
	gate := graphMatmulParityGate{ // want `graph/compiler-fused matmul parity manifest is incomplete; matching visible dtypes do not prove matching accumulator/reduction semantics; missing broad/large-K production shape`
		ExplicitConversionControlRoute: "explicit f16 casts plus vendor matmul",
		GraphFusedCandidateRoute:       "compiler-fused graph cast-matmul-cast",
		SmallShape:                     shape{M: 33, K: 512, N: 257},
	}
	_ = gate
	for range b.N {
		graphFusedMatmul()
	}
}

func BenchmarkGraphMatmulPromotionWeakParity(b *testing.B) {
	gate := graphMatmulParityGate{ // want `graph/compiler-fused matmul parity evidence is invalid: same-shape comparator is disabled or checks visible dtype only; finite-output gate is explicitly disabled`
		ExplicitConversionControlRoute: "explicit f16 casts plus vendor matmul",
		GraphFusedCandidateRoute:       "compiler-fused graph cast-matmul-cast",
		SmallShape:                     shape{M: 33, K: 512, N: 257},
		BroadProductionShape:           shape{M: 64, K: 5632, N: 2048},
		SmallSameShapeComparator:       "visible dtype only",
		ProductionSameShapeComparator:  "candidate vs reference output",
		FiniteOutputGate:               false,
		RelativeErrorTolerance:         1e-4,
	}
	_ = gate
	for range b.N {
		graphFusedMatmul()
	}
}

func BenchmarkGraphMatmulPromotionComplete(b *testing.B) {
	gate := graphMatmulParityGate{
		ExplicitConversionControlRoute: "explicit f16 casts plus vendor matmul",
		GraphFusedCandidateRoute:       "compiler-fused graph cast-matmul-cast",
		SmallShape:                     shape{M: 33, K: 512, N: 257},
		BroadProductionShape:           shape{M: 64, K: 5632, N: 2048},
		SmallSameShapeComparator:       "candidate vs same-shape reference output",
		ProductionSameShapeComparator:  "candidate vs same-shape reference output",
		FiniteOutputGate:               true,
		RelativeErrorTolerance:         1e-4,
	}
	_ = gate
	for range b.N {
		graphFusedMatmul()
	}
}

// Graph compilation alone is not a cast-matmul route comparison.
func BenchmarkGraphCompilerCache(b *testing.B) {
	for range b.N {
		graphFusedMatmul()
	}
}

// Ordinary correctness tests stay outside the promotion-harness audit.
func TestGraphMatmulCorrectness(t *testing.T) {
	explicitCastVendorMatmul()
	graphFusedMatmul()
}
