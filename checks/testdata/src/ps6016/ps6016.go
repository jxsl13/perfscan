package ps6016

import "testing"

type AmdahlFusionGate struct {
	Hardware               string
	Workload               string
	ProfiledStageShare     float64
	DispatchBefore         int
	DispatchAfter          int
	LeafRatios             []float64
	DecodeCandidateRatios  []float64
	UnchangedControlRatios []float64
	DecodeRequiredRatio    float64
	ShortPrefillRatios     []float64
	ShortPrefillMinimum    float64
	LongPrefillRatios      []float64
	LongPrefillMinimum     float64
	ExactnessPassed        bool
}

type FusionAmdahlEvidence struct {
	Hardware                  string
	Workload                  string
	ProfiledStageSharePercent float64
	EncoderControl            int
	EncoderFused              int
	LeafSpeedupDistribution   []float64
	TG64CandidateSamples      []float64
	ControlRatioDistribution  []float64
	TG64Required              float64
	PP64Samples               []float64
	PP64Minimum               float64
	PP512Samples              []float64
	PP512Minimum              float64
	BitExactStatus            string
}

type LeafToGraphGate struct{}

// A narrower leaf-to-graph evidence manifest is handled by PS6017 rather than
// being diagnosed as a missing PS6016 Amdahl/prefill manifest.
func BenchmarkMetalDispatchFusionDecodeLeafGate(b *testing.B) {
	_ = LeafToGraphGate{}
}

func BenchmarkMetalDispatchFusionAmdahlMissing(b *testing.B) { // want "dispatch-eliminating accelerator fusion has no Amdahl-aware graph gate manifest; missing hardware"
	b.ReportAllocs()
}

func BenchmarkMetalDispatchFusionAmdahlIncomplete(b *testing.B) {
	_ = AmdahlFusionGate{ // want "Amdahl fusion gate manifest is incomplete; missing workload.*exactness status"
		Hardware: "M2 Max",
	}
}

func BenchmarkMetalDispatchFusionAmdahlIssue769(b *testing.B) {
	_ = AmdahlFusionGate{ // want "profiled share 3.80% gives full-removal ceiling 1.04x.*decode invocation 1.014x misses frozen 1.02x gate.*short-prefill invocation 0.9876x misses 0.99x regression minimum.*decode threshold 1.02x is within control spread"
		Hardware:               "M2 Max",
		Workload:               "trained model tg64 pp64 pp512",
		ProfiledStageShare:     0.038,
		DispatchBefore:         44,
		DispatchAfter:          0,
		LeafRatios:             []float64{1.023, 1.023, 1.013},
		DecodeCandidateRatios:  []float64{1.0235, 1.0206, 1.0140},
		UnchangedControlRatios: []float64{1.0, 1.0241, 1.01},
		DecodeRequiredRatio:    1.02,
		ShortPrefillRatios:     []float64{0.9876, 1.0121, 1.0104},
		ShortPrefillMinimum:    0.99,
		LongPrefillRatios:      []float64{1.0052, 1.0120, 1.0034},
		LongPrefillMinimum:     0.99,
		ExactnessPassed:        true,
	}
}

func BenchmarkMetalDispatchFusionAmdahlHealthy(b *testing.B) {
	_ = AmdahlFusionGate{
		Hardware:               "test accelerator",
		Workload:               "decode and prefill graph",
		ProfiledStageShare:     0.10,
		DispatchBefore:         20,
		DispatchAfter:          2,
		LeafRatios:             []float64{1.08, 1.09, 1.10},
		DecodeCandidateRatios:  []float64{1.09, 1.10, 1.11},
		UnchangedControlRatios: []float64{1.00, 1.01, 1.02},
		DecodeRequiredRatio:    1.05,
		ShortPrefillRatios:     []float64{1.00, 1.01, 1.02},
		ShortPrefillMinimum:    0.99,
		LongPrefillRatios:      []float64{1.00, 1.01, 1.02},
		LongPrefillMinimum:     0.99,
		ExactnessPassed:        true,
	}
}

func BenchmarkMetalDispatchFusionAmdahlPercentShare(b *testing.B) {
	_ = FusionAmdahlEvidence{
		Hardware:                  "test accelerator",
		Workload:                  "decode and prefill graph",
		ProfiledStageSharePercent: 10,
		EncoderControl:            20,
		EncoderFused:              2,
		LeafSpeedupDistribution:   []float64{1.08, 1.09, 1.10},
		TG64CandidateSamples:      []float64{1.09, 1.10, 1.11},
		ControlRatioDistribution:  []float64{1.00, 1.01, 1.02},
		TG64Required:              1.05,
		PP64Samples:               []float64{1.00, 1.01, 1.02},
		PP64Minimum:               0.99,
		PP512Samples:              []float64{1.00, 1.01, 1.02},
		PP512Minimum:              0.99,
		BitExactStatus:            "passed",
	}
}

func BenchmarkMetalDispatchFusionAmdahlShortInvalid(b *testing.B) {
	_ = AmdahlFusionGate{ // want "dispatch/encoder count does not validly decrease.*exactness/parity gate explicitly fails.*leaf campaign has fewer than three independent invocations.*long-prefill campaign has fewer than three independent invocations"
		Hardware:               "test accelerator",
		Workload:               "decode and prefill graph",
		ProfiledStageShare:     0.10,
		DispatchBefore:         20,
		DispatchAfter:          20,
		LeafRatios:             []float64{1.08, 1.09},
		DecodeCandidateRatios:  []float64{1.07, 1.08},
		UnchangedControlRatios: []float64{1.00, 1.01},
		DecodeRequiredRatio:    1.05,
		ShortPrefillRatios:     []float64{1.00, 1.01},
		ShortPrefillMinimum:    0.99,
		LongPrefillRatios:      []float64{1.00, 1.01},
		LongPrefillMinimum:     0.99,
		ExactnessPassed:        false,
	}
}

func BenchmarkMetalDispatchFusionAmdahlDynamic(b *testing.B) {
	leaf, decode, controls := ratios(), ratios(), ratios()
	shortPrefill, longPrefill := ratios(), ratios()
	_ = AmdahlFusionGate{
		Hardware:               "test accelerator",
		Workload:               "decode and prefill graph",
		ProfiledStageShare:     dynamicRatio(),
		DispatchBefore:         dynamicCount(),
		DispatchAfter:          dynamicCount(),
		LeafRatios:             leaf,
		DecodeCandidateRatios:  decode,
		UnchangedControlRatios: controls,
		DecodeRequiredRatio:    dynamicRatio(),
		ShortPrefillRatios:     shortPrefill,
		ShortPrefillMinimum:    dynamicRatio(),
		LongPrefillRatios:      longPrefill,
		LongPrefillMinimum:     dynamicRatio(),
		ExactnessPassed:        dynamicBool(),
	}
}

func ratios() []float64     { return nil }
func dynamicRatio() float64 { return 0 }
func dynamicCount() int     { return 0 }
func dynamicBool() bool     { return false }
