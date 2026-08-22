package ps6017

import "testing"

type FusionLeverageEvidence struct {
	Hardware                string
	Workload                string
	LeafSpeedup             float64
	EndToEndCandidateRatios []float64
	UnchangedControlRatios  []float64
	GraphPromotionThreshold float64
	MaximumControlSpread    float64
	ExactnessPassed         bool
}

type LeafToGraphGate struct {
	Device                      string
	Model                       string
	LeafRatioDistribution       []float64
	DecodeCandidateSamples      []float64
	ControlRatioDistribution    []float64
	DecodeRequired              float64
	MaximumControlSpreadPercent float64
	BitExactStatus              string
}

func BenchmarkMetalQKVFusionLeafGraphMissing(b *testing.B) { // want "accelerator leaf-fusion campaign has no leaf-to-graph leverage manifest; missing hardware"
	b.ReportAllocs()
}

func BenchmarkMetalQKVFusionLeafGraphIncomplete(b *testing.B) {
	_ = FusionLeverageEvidence{ // want "leaf-to-graph leverage manifest is incomplete; missing workload.*exactness status"
		Hardware: "Apple M2",
	}
}

func BenchmarkMetalQKVFusionLeafGraphIssue768(b *testing.B) {
	_ = FusionLeverageEvidence{ // want "leaf median 1.381x versus end-to-end 1.009x-1.017x implies 3.23%-6.09% effective stage fraction.*best end-to-end candidate 1.017x cannot clear frozen 1.03x promotion threshold.*leaf gain 38.15% diverges from end-to-end median gain 1.19% beyond 2.41% control spread"
		Hardware:                "Apple M2 Metal",
		Workload:                "TinyLlama Q4_K_M tg64 graph",
		LeafSpeedup:             1.3815,
		EndToEndCandidateRatios: []float64{1.0171, 1.0119, 1.0090},
		UnchangedControlRatios:  []float64{1.0, 1.0241, 1.01},
		GraphPromotionThreshold: 1.03,
		MaximumControlSpread:    0.025,
		ExactnessPassed:         true,
	}
}

func BenchmarkMetalQKVFusionLeafGraphHealthy(b *testing.B) {
	_ = FusionLeverageEvidence{
		Hardware:                "test accelerator",
		Workload:                "decode graph",
		LeafSpeedup:             1.09,
		EndToEndCandidateRatios: []float64{1.07, 1.08, 1.09},
		UnchangedControlRatios:  []float64{1.0, 1.01, 1.02},
		GraphPromotionThreshold: 1.05,
		MaximumControlSpread:    0.03,
		ExactnessPassed:         true,
	}
}

func BenchmarkMetalQKVFusionLeafGraphPercentLimit(b *testing.B) {
	_ = LeafToGraphGate{
		Device:                      "test accelerator",
		Model:                       "decode graph",
		LeafRatioDistribution:       []float64{1.08, 1.09, 1.10},
		DecodeCandidateSamples:      []float64{1.07, 1.08, 1.09},
		ControlRatioDistribution:    []float64{1.0, 1.01, 1.02},
		DecodeRequired:              1.05,
		MaximumControlSpreadPercent: 3,
		BitExactStatus:              "passed",
	}
}

func BenchmarkMetalQKVFusionLeafGraphUnstableShort(b *testing.B) {
	_ = FusionLeverageEvidence{ // want "exactness/parity gate explicitly fails.*end-to-end candidate campaign has fewer than three independent invocations.*unchanged-control campaign has fewer than three independent invocations.*unchanged-control spread 10.00% exceeds declared 2.00% limit"
		Hardware:                "test accelerator",
		Workload:                "decode graph",
		LeafSpeedup:             1.20,
		EndToEndCandidateRatios: []float64{1.05, 1.06},
		UnchangedControlRatios:  []float64{1.0, 1.10},
		GraphPromotionThreshold: 1.04,
		MaximumControlSpread:    0.02,
		ExactnessPassed:         false,
	}
}

func BenchmarkMetalQKVFusionLeafGraphDynamic(b *testing.B) {
	_ = FusionLeverageEvidence{
		Hardware:                "test accelerator",
		Workload:                "decode graph",
		LeafSpeedup:             dynamicRatio(),
		EndToEndCandidateRatios: dynamicRatios(),
		UnchangedControlRatios:  dynamicRatios(),
		GraphPromotionThreshold: dynamicRatio(),
		MaximumControlSpread:    dynamicRatio(),
		ExactnessPassed:         dynamicBool(),
	}
}

func dynamicRatios() []float64 { return nil }
func dynamicRatio() float64    { return 0 }
func dynamicBool() bool        { return false }
