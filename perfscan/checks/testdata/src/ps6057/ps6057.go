package ps6057

import "testing"

type Tensor struct{}

type MetalRecorder struct{}

func (*MetalRecorder) Q4KMatmul(input, weight *Tensor) *Tensor      { return input }
func (*MetalRecorder) Q6KMatmul(input, weight *Tensor) *Tensor      { return input }
func (*MetalRecorder) FloatMatmul(input, weight *Tensor) *Tensor    { return input }
func (*MetalRecorder) FusedQ4KMatmul(input, weight *Tensor) *Tensor { return input }
func (*MetalRecorder) ResidualAdd(input, residual *Tensor) *Tensor  { return input }
func (*MetalRecorder) SwiGLU(gate, up *Tensor) *Tensor              { return gate }
func (*MetalRecorder) Consume(inputs ...*Tensor)                    {}

type CPUOps struct{}

func (*CPUOps) Q4KMatmul(input, weight *Tensor) *Tensor     { return input }
func (*CPUOps) ResidualAdd(input, residual *Tensor) *Tensor { return input }

func repeatedMetalQ4KResidual(rec *MetalRecorder, input, weight, residual *Tensor) {
	first := rec.Q4KMatmul(input, weight) // want `static GPU recorder graph repeats Q4KMatmul -> ResidualAdd 2 times with adjacent single-consumer intermediates`
	out1 := rec.ResidualAdd(first, residual)
	second := rec.Q4KMatmul(input, weight)
	out2 := rec.ResidualAdd(second, residual)
	rec.Consume(out1, out2)
}

func repeatedMetalPairedQ4KSwiGLU(rec *MetalRecorder, input, gateWeight, upWeight *Tensor) {
	gate1 := rec.Q4KMatmul(input, gateWeight) // want `static GPU recorder graph repeats Q4KMatmul\+Q4KMatmul -> SwiGLU 2 times with adjacent single-consumer intermediates`
	up1 := rec.Q4KMatmul(input, upWeight)
	out1 := rec.SwiGLU(gate1, up1)
	gate2 := rec.Q4KMatmul(input, gateWeight)
	up2 := rec.Q4KMatmul(input, upWeight)
	out2 := rec.SwiGLU(gate2, up2)
	rec.Consume(out1, out2)
}

// A lone boundary is not the repeated candidate this rule targets.
func singleMetalQ4KResidual(rec *MetalRecorder, input, weight, residual *Tensor) *Tensor {
	tmp := rec.Q4KMatmul(input, weight)
	return rec.ResidualAdd(tmp, residual)
}

// Reuse makes the producer output non-single-consumer.
func reusedMetalQ4KResidual(rec *MetalRecorder, input, weight, residual *Tensor) {
	first := rec.Q4KMatmul(input, weight)
	out1 := rec.ResidualAdd(first, residual)
	rec.Consume(first)
	second := rec.Q4KMatmul(input, weight)
	out2 := rec.ResidualAdd(second, residual)
	rec.Consume(second, out1, out2)
}

// Different command contexts are not an adjacent fusible seam.
func differentRecorders(first, second *MetalRecorder, input, weight, residual *Tensor) {
	a := first.Q4KMatmul(input, weight)
	b := second.ResidualAdd(a, residual)
	c := first.Q4KMatmul(input, weight)
	d := second.ResidualAdd(c, residual)
	first.Consume(b, d)
}

// Ordinary CPU helpers and non-quantized/fused producers stay silent.
func nonCandidates(cpu *CPUOps, rec *MetalRecorder, input, weight, residual *Tensor) {
	a := cpu.Q4KMatmul(input, weight)
	b := cpu.ResidualAdd(a, residual)
	c := rec.FloatMatmul(input, weight)
	d := rec.ResidualAdd(c, residual)
	e := rec.FusedQ4KMatmul(input, weight)
	f := rec.ResidualAdd(e, residual)
	rec.Consume(b, d, f)
}

var _ = []any{repeatedMetalQ4KResidual, repeatedMetalPairedQ4KSwiGLU, singleMetalQ4KResidual, reusedMetalQ4KResidual, differentRecorders, nonCandidates}

type stableEncoderBoundaryEvidence struct {
	Hardware                       string
	WorkloadIdentity               string
	EvidenceSource                 string
	StableEncoderLabels            bool
	CommandEncoderCount            int
	CommandDurationNS              float64
	TotalEncoderIntervalNS         float64
	IntervalsIncludeDependencyWait bool
	ExclusiveKernelTimeClaimed     bool
	EncoderGroupLabels             []string
	EncoderGroupOccurrenceCounts   []int
	EncoderGroupCoveredTimesNS     []float64
	BoundaryChainLabels            []string
	BoundaryOccurrenceCounts       []int
	BoundaryCoveredTimesNS         []float64
	RepeatedBoundaryTotalCount     int
	RepeatedBoundaryCoveredTimeNS  float64
	CoveredCommandTimeFraction     float64
	FusionExperimentRecommended    bool
	EndToEndBenchmarkRequired      bool
	EndToEndBenchmarkCompleted     bool
	EndToEndControlNS              float64
	EndToEndCandidateNS            float64
	EndToEndControlCandidateRatio  float64
	PromotionThreshold             float64
	CandidatePromoted              bool
	PriorNegativeFusionDisclosed   bool
	CandidateClassification        string
	FinalDecision                  string
}

func runMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventory() {}

func TestMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventoryMissing(t *testing.T) { // want `GPU producer/elementwise label inventory has no boundary manifest`
	runMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventory()
}

func TestMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventoryIncomplete(t *testing.T) {
	evidence := stableEncoderBoundaryEvidence{ // want `GPU producer/elementwise boundary evidence is incomplete; missing workload identity, evidence source, stable encoder labels`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventory()
}

func TestMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventoryOverclaim(t *testing.T) {
	evidence := stableEncoderBoundaryEvidence{ // want `dependency-charged encoder intervals are claimed as exclusive kernel time; fusion candidate is promoted without a completed end-to-end benchmark above threshold; final decision "retained" treats boundary attribution as a proven performance win`
		Hardware:                       "Apple M2 Pro",
		WorkloadIdentity:               "TinyLlama-1.1B Q4_K_M one-command-buffer decode",
		EvidenceSource:                 "stable GPU encoder labels",
		StableEncoderLabels:            true,
		CommandEncoderCount:            340,
		CommandDurationNS:              37_970_000,
		TotalEncoderIntervalNS:         35_700_000,
		IntervalsIncludeDependencyWait: true,
		ExclusiveKernelTimeClaimed:     true,
		EncoderGroupLabels:             []string{"binary boundaries", "Q4_K matmuls", "decode attention", "Q6_K matmuls", "remaining groups"},
		EncoderGroupOccurrenceCounts:   []int{66, 110, 22, 21, 121},
		EncoderGroupCoveredTimesNS:     []float64{26_900_000, 4_370_000, 2_310_000, 1_260_000, 860_000},
		BoundaryChainLabels:            []string{"Q4_K matmul -> residual add", "paired quantized matmuls -> SwiGLU"},
		BoundaryOccurrenceCounts:       []int{44, 22},
		BoundaryCoveredTimesNS:         []float64{17_933_333, 8_966_667},
		RepeatedBoundaryTotalCount:     66,
		RepeatedBoundaryCoveredTimeNS:  26_900_000,
		CoveredCommandTimeFraction:     26_900_000.0 / 37_970_000,
		FusionExperimentRecommended:    true,
		EndToEndBenchmarkRequired:      true,
		EndToEndBenchmarkCompleted:     false,
		EndToEndControlNS:              0,
		EndToEndCandidateNS:            0,
		EndToEndControlCandidateRatio:  0,
		PromotionThreshold:             1.10,
		CandidatePromoted:              true,
		PriorNegativeFusionDisclosed:   true,
		CandidateClassification:        "proven-performance-win",
		FinalDecision:                  "retained",
	}
	_ = evidence
	runMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventory()
}

func TestMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventoryStale(t *testing.T) {
	evidence := stableEncoderBoundaryEvidence{ // want `covered command-time fraction 1 disagrees with 0.708454`
		Hardware:                       "Apple M2 Pro",
		WorkloadIdentity:               "TinyLlama-1.1B Q4_K_M one-command-buffer decode",
		EvidenceSource:                 "stable GPU encoder labels",
		StableEncoderLabels:            true,
		CommandEncoderCount:            340,
		CommandDurationNS:              37_970_000,
		TotalEncoderIntervalNS:         35_700_000,
		IntervalsIncludeDependencyWait: true,
		ExclusiveKernelTimeClaimed:     false,
		EncoderGroupLabels:             []string{"binary boundaries", "Q4_K matmuls", "decode attention", "Q6_K matmuls", "remaining groups"},
		EncoderGroupOccurrenceCounts:   []int{66, 110, 22, 21, 121},
		EncoderGroupCoveredTimesNS:     []float64{26_900_000, 4_370_000, 2_310_000, 1_260_000, 860_000},
		BoundaryChainLabels:            []string{"Q4_K matmul -> residual add", "paired quantized matmuls -> SwiGLU"},
		BoundaryOccurrenceCounts:       []int{44, 22},
		BoundaryCoveredTimesNS:         []float64{17_933_333, 8_966_667},
		RepeatedBoundaryTotalCount:     66,
		RepeatedBoundaryCoveredTimeNS:  26_900_000,
		CoveredCommandTimeFraction:     1,
		FusionExperimentRecommended:    true,
		EndToEndBenchmarkRequired:      true,
		EndToEndBenchmarkCompleted:     false,
		EndToEndControlNS:              0,
		EndToEndCandidateNS:            0,
		EndToEndControlCandidateRatio:  0,
		PromotionThreshold:             1.10,
		CandidatePromoted:              false,
		PriorNegativeFusionDisclosed:   true,
		CandidateClassification:        "boundary-candidate-not-performance-proof",
		FinalDecision:                  "candidate-only",
	}
	_ = evidence
	runMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventory()
}

func TestMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventoryStable(t *testing.T) {
	evidence := stableEncoderBoundaryEvidence{
		Hardware:                       "Apple M2 Pro",
		WorkloadIdentity:               "TinyLlama-1.1B Q4_K_M one-command-buffer decode",
		EvidenceSource:                 "stable GPU encoder labels",
		StableEncoderLabels:            true,
		CommandEncoderCount:            340,
		CommandDurationNS:              37_970_000,
		TotalEncoderIntervalNS:         35_700_000,
		IntervalsIncludeDependencyWait: true,
		ExclusiveKernelTimeClaimed:     false,
		EncoderGroupLabels:             []string{"binary boundaries", "Q4_K matmuls", "decode attention", "Q6_K matmuls", "remaining groups"},
		EncoderGroupOccurrenceCounts:   []int{66, 110, 22, 21, 121},
		EncoderGroupCoveredTimesNS:     []float64{26_900_000, 4_370_000, 2_310_000, 1_260_000, 860_000},
		BoundaryChainLabels:            []string{"Q4_K matmul -> residual add", "paired quantized matmuls -> SwiGLU"},
		BoundaryOccurrenceCounts:       []int{44, 22},
		BoundaryCoveredTimesNS:         []float64{17_933_333, 8_966_667},
		RepeatedBoundaryTotalCount:     66,
		RepeatedBoundaryCoveredTimeNS:  26_900_000,
		CoveredCommandTimeFraction:     26_900_000.0 / 37_970_000,
		FusionExperimentRecommended:    true,
		EndToEndBenchmarkRequired:      true,
		EndToEndBenchmarkCompleted:     false,
		EndToEndControlNS:              0,
		EndToEndCandidateNS:            0,
		EndToEndControlCandidateRatio:  0,
		PromotionThreshold:             1.10,
		CandidatePromoted:              false,
		PriorNegativeFusionDisclosed:   true,
		CandidateClassification:        "boundary-candidate-not-performance-proof",
		FinalDecision:                  "candidate-only",
	}
	_ = evidence
	runMetalGPUStableEncoderLabelProducerElementwiseBoundaryCandidateInventory()
}
