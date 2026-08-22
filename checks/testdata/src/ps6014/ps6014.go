package ps6014

import "testing"

func runGroupedMPSFusion() {}

func BenchmarkMetalGroupedStructuralLatencyFusion(b *testing.B) { // want "structural accelerator-fusion benchmark has no separate structural/latency leverage manifest; missing hardware, production shape"
	for range b.N {
		runGroupedMPSFusion()
	}
}

type shape struct {
	M int
	K int
	N int
}

type structuralLatencyGate struct {
	Hardware         string
	ProductionShape  shape
	DType            string
	WarmColdState    string
	StructuralMetric string
	StructuralBefore int
	StructuralAfter  int
	BenchmarkSamples int
	MedianRatio      float64
	RequiredRatio    float64
	ExactnessPassed  bool
}

type fusionLeverageGate struct {
	Hardware         string
	ProductionShape  shape
	DType            string
	WarmColdState    string
	StructuralMetric string
	StructuralBefore int
	StructuralAfter  int
	BenchmarkSamples []float64
	MedianRatio      float64
	RequiredRatio    float64
	ExactnessStatus  string
}

func BenchmarkMetalFusionLeveragePartial(b *testing.B) {
	gate := structuralLatencyGate{ // want "structural/latency fusion manifest is incomplete; missing exactness status"
		Hardware:         "Apple M2 Pro",
		ProductionShape:  shape{M: 64, K: 2048, N: 5632},
		DType:            "cached f16 / Q4_K",
		WarmColdState:    "warm",
		StructuralMetric: "MPS projections",
		StructuralBefore: 2,
		StructuralAfter:  1,
		BenchmarkSamples: 10,
		MedianRatio:      1.021,
		RequiredRatio:    1.08,
	}
	_ = gate
	for range b.N {
		runGroupedMPSFusion()
	}
}

func BenchmarkMetalFusionLeverageLow(b *testing.B) {
	gate := structuralLatencyGate{ // want "structural/latency fusion evidence fails independent gates: structural count improves 2 -> 1 but median ratio 1.021x misses the declared 1.08x floor; classify this as low leverage/false proxy, not a performance win"
		Hardware:         "Apple M2 Pro",
		ProductionShape:  shape{M: 64, K: 2048, N: 5632},
		DType:            "cached f16 / Q4_K",
		WarmColdState:    "warm cache",
		StructuralMetric: "MPS projections",
		StructuralBefore: 2,
		StructuralAfter:  1,
		BenchmarkSamples: 10,
		MedianRatio:      1.021,
		RequiredRatio:    1.08,
		ExactnessPassed:  true,
	}
	_ = gate
	for range b.N {
		runGroupedMPSFusion()
	}
}

func BenchmarkMetalFusionLeveragePass(b *testing.B) {
	gate := structuralLatencyGate{
		Hardware:         "Apple M2 Pro",
		ProductionShape:  shape{M: 64, K: 2048, N: 5632},
		DType:            "cached f16 / Q4_K",
		WarmColdState:    "warm cache",
		StructuralMetric: "MPS projections",
		StructuralBefore: 2,
		StructuralAfter:  1,
		BenchmarkSamples: 10,
		MedianRatio:      1.11,
		RequiredRatio:    1.08,
		ExactnessPassed:  true,
	}
	_ = gate
	for range b.N {
		runGroupedMPSFusion()
	}
}

func BenchmarkVulkanFusionSampleVector(b *testing.B) {
	gate := fusionLeverageGate{ // want "structural count improves 4 -> 2 but median ratio 1.01x misses the declared 1.05x floor"
		Hardware:         "Vulkan test GPU",
		ProductionShape:  shape{M: 64, K: 2048, N: 5632},
		DType:            "f16",
		WarmColdState:    "warm",
		StructuralMetric: "dispatches",
		StructuralBefore: 4,
		StructuralAfter:  2,
		BenchmarkSamples: []float64{1.00, 1.01, 1.02},
		MedianRatio:      1.01,
		RequiredRatio:    1.05,
		ExactnessStatus:  "bit-exact pass",
	}
	_ = gate
	for range b.N {
		runGroupedMPSFusion()
	}
}

func BenchmarkCUDAFusionNoStructuralReduction(b *testing.B) {
	gate := structuralLatencyGate{ // want "claimed structural win does not reduce the count [(]2 -> 2[)]"
		Hardware:         "CUDA test GPU",
		ProductionShape:  shape{M: 64, K: 2048, N: 5632},
		DType:            "f16",
		WarmColdState:    "warm",
		StructuralMetric: "kernel launches",
		StructuralBefore: 2,
		StructuralAfter:  2,
		BenchmarkSamples: 10,
		MedianRatio:      1.2,
		RequiredRatio:    1.08,
		ExactnessPassed:  true,
	}
	_ = gate
	for range b.N {
		runGroupedMPSFusion()
	}
}

func BenchmarkMetalFusionExactnessFailure(b *testing.B) {
	gate := structuralLatencyGate{ // want "exactness/parity gate explicitly fails"
		Hardware:         "Apple M2 Pro",
		ProductionShape:  shape{M: 64, K: 2048, N: 5632},
		DType:            "f16",
		WarmColdState:    "warm",
		StructuralMetric: "encoders",
		StructuralBefore: 2,
		StructuralAfter:  1,
		BenchmarkSamples: 10,
		MedianRatio:      1.2,
		RequiredRatio:    1.08,
		ExactnessPassed:  false,
	}
	_ = gate
	for range b.N {
		runGroupedMPSFusion()
	}
}

func BenchmarkMetalFusionNoSamples(b *testing.B) {
	gate := structuralLatencyGate{ // want "benchmark sample count is not positive"
		Hardware:         "Apple M2 Pro",
		ProductionShape:  shape{M: 64, K: 2048, N: 5632},
		DType:            "f16",
		WarmColdState:    "warm",
		StructuralMetric: "encoders",
		StructuralBefore: 2,
		StructuralAfter:  1,
		BenchmarkSamples: 0,
		MedianRatio:      1.2,
		RequiredRatio:    1.08,
		ExactnessPassed:  true,
	}
	_ = gate
	for range b.N {
		runGroupedMPSFusion()
	}
}

func BenchmarkMetalFusionDynamicEvidence(b *testing.B) {
	before, after := structuralCounts()
	median, required := latencyRatios()
	gate := structuralLatencyGate{
		Hardware:         hardwareName(),
		ProductionShape:  productionShape(),
		DType:            dtypeName(),
		WarmColdState:    timingState(),
		StructuralMetric: metricName(),
		StructuralBefore: before,
		StructuralAfter:  after,
		BenchmarkSamples: sampleCount(),
		MedianRatio:      median,
		RequiredRatio:    required,
		ExactnessPassed:  exactnessPassed(),
	}
	_ = gate
	for range b.N {
		runGroupedMPSFusion()
	}
}

func TestMetalFusionPromotionGate(t *testing.T) { // want "structural accelerator-fusion benchmark has no separate structural/latency leverage manifest"
	runGroupedMPSFusion()
	_ = "structural projection count median ratio"
}

func BenchmarkMetalProjectionLatency(b *testing.B) {
	for range b.N {
		runGroupedMPSFusion()
	}
}

func structuralCounts() (int, int)      { return 2, 1 }
func latencyRatios() (float64, float64) { return 1.1, 1.08 }
func hardwareName() string              { return "GPU" }
func productionShape() shape            { return shape{} }
func dtypeName() string                 { return "f16" }
func timingState() string               { return "warm" }
func metricName() string                { return "dispatches" }
func sampleCount() int                  { return 10 }
func exactnessPassed() bool             { return true }
