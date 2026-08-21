package ps6038

import "testing"

type recorderLeverageEvidence struct {
	Hardware                            string
	WorkloadShape                       string
	WarmColdState                       string
	ControlCommandBufferOwnershipMode   string
	CandidateCommandBufferOwnershipMode string
	LeafWorkload                        string
	EncodersPerCommand                  int
	LeafControlEncodeNS                 float64
	LeafCandidateEncodeNS               float64
	LeafSavedEncodeNS                   float64
	LeafSpeedup                         float64
	ControlGPUCommandDurationNS         float64
	CandidateGPUCommandDurationNS       float64
	ControlSynchronizationTimeNS        float64
	CandidateSynchronizationTimeNS      float64
	ControlTotalWallNS                  float64
	CandidateTotalWallNS                float64
	ParentWorkload                      string
	ParentControlWallNS                 float64
	ParentCandidateWallNS               float64
	ParentThroughputRatio               float64
	LeverageFraction                    float64
	MaterialityThreshold                float64
	ParentPromotionGate                 float64
	ExactOutputDigestPassed             bool
	ControlAllocationBytes              int
	CandidateAllocationBytes            int
	MixedComputeBlitLifetimePassed      bool
	ProfilingRecorderCompatible         bool
	ParentGateRun                       bool
	Classification                      string
	FinalDecision                       string
}

func runMetalHostOnlyRecorderParentLeverage() {}

func TestMetalHostOnlyRecorderParentLeverageMissing(t *testing.T) { // want `host-recorder/parent campaign has no leverage manifest`
	runMetalHostOnlyRecorderParentLeverage()
}

func TestMetalHostOnlyRecorderParentLeverageIncomplete(t *testing.T) {
	evidence := recorderLeverageEvidence{ // want `host-recorder leverage evidence is incomplete; missing workload shape, warm/cold state, control ownership mode`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalHostOnlyRecorderParentLeverage()
}

func TestMetalHostOnlyRecorderParentLeverageOverclaimed(t *testing.T) {
	evidence := recorderLeverageEvidence{ // want `classification "application-leverage" is not host-only despite .* leverage below 0.001 materiality; final decision "retained" retains a candidate below 1.03x parent gate`
		Hardware:                            "Apple M2 Pro",
		WorkloadShape:                       "32 custom compute encoders",
		WarmColdState:                       "warm",
		ControlCommandBufferOwnershipMode:   "retained references",
		CandidateCommandBufferOwnershipMode: "unretained references",
		LeafWorkload:                        "host-only command construction",
		EncodersPerCommand:                  32,
		LeafControlEncodeNS:                 106829,
		LeafCandidateEncodeNS:               20656,
		LeafSavedEncodeNS:                   86173,
		LeafSpeedup:                         106829.0 / 20656,
		ControlGPUCommandDurationNS:         1_000_000,
		CandidateGPUCommandDurationNS:       1_000_000,
		ControlSynchronizationTimeNS:        1000,
		CandidateSynchronizationTimeNS:      1000,
		ControlTotalWallNS:                  1_106_829,
		CandidateTotalWallNS:                1_020_656,
		ParentWorkload:                      "TinyLlama-1.1B Q4_K_M 200-token decode",
		ParentControlWallNS:                 7_336_289_041,
		ParentCandidateWallNS:               7_352_295_125,
		ParentThroughputRatio:               7_336_289_041.0 / 7_352_295_125,
		LeverageFraction:                    86173.0 / 7_336_289_041,
		MaterialityThreshold:                0.001,
		ParentPromotionGate:                 1.03,
		ExactOutputDigestPassed:             true,
		ControlAllocationBytes:              8,
		CandidateAllocationBytes:            8,
		MixedComputeBlitLifetimePassed:      true,
		ProfilingRecorderCompatible:         true,
		ParentGateRun:                       true,
		Classification:                      "application-leverage",
		FinalDecision:                       "retained",
	}
	_ = evidence
	runMetalHostOnlyRecorderParentLeverage()
}

func TestMetalHostOnlyRecorderParentLeverageRatioMismatch(t *testing.T) {
	evidence := recorderLeverageEvidence{ // want `leverage fraction 0.1 disagrees with saved-leaf/parent-wall`
		Hardware:                            "Apple M2 Pro",
		WorkloadShape:                       "32 custom compute encoders",
		WarmColdState:                       "warm",
		ControlCommandBufferOwnershipMode:   "retained references",
		CandidateCommandBufferOwnershipMode: "unretained references",
		LeafWorkload:                        "host-only command construction",
		EncodersPerCommand:                  32,
		LeafControlEncodeNS:                 106829,
		LeafCandidateEncodeNS:               20656,
		LeafSavedEncodeNS:                   86173,
		LeafSpeedup:                         106829.0 / 20656,
		ControlGPUCommandDurationNS:         1_000_000,
		CandidateGPUCommandDurationNS:       1_000_000,
		ControlSynchronizationTimeNS:        1000,
		CandidateSynchronizationTimeNS:      1000,
		ControlTotalWallNS:                  1_106_829,
		CandidateTotalWallNS:                1_020_656,
		ParentWorkload:                      "TinyLlama-1.1B Q4_K_M 200-token decode",
		ParentControlWallNS:                 7_336_289_041,
		ParentCandidateWallNS:               7_352_295_125,
		ParentThroughputRatio:               7_336_289_041.0 / 7_352_295_125,
		LeverageFraction:                    0.1,
		MaterialityThreshold:                0.001,
		ParentPromotionGate:                 1.03,
		ExactOutputDigestPassed:             true,
		ControlAllocationBytes:              8,
		CandidateAllocationBytes:            8,
		MixedComputeBlitLifetimePassed:      true,
		ProfilingRecorderCompatible:         true,
		ParentGateRun:                       true,
		Classification:                      "host-only",
		FinalDecision:                       "removed",
	}
	_ = evidence
	runMetalHostOnlyRecorderParentLeverage()
}

func TestMetalHostOnlyRecorderParentLeverageFalseGates(t *testing.T) {
	evidence := recorderLeverageEvidence{ // want `exact-output digest is explicitly false; mixed compute/blit lifetime is explicitly false; profiling-recorder compatibility is explicitly false; parent-gate execution is explicitly false`
		Hardware:                            "Apple M2 Pro",
		WorkloadShape:                       "32 custom compute encoders",
		WarmColdState:                       "warm",
		ControlCommandBufferOwnershipMode:   "retained references",
		CandidateCommandBufferOwnershipMode: "unretained references",
		LeafWorkload:                        "host-only command construction",
		EncodersPerCommand:                  32,
		LeafControlEncodeNS:                 106829,
		LeafCandidateEncodeNS:               20656,
		LeafSavedEncodeNS:                   86173,
		LeafSpeedup:                         106829.0 / 20656,
		ControlGPUCommandDurationNS:         1_000_000,
		CandidateGPUCommandDurationNS:       1_000_000,
		ControlSynchronizationTimeNS:        1000,
		CandidateSynchronizationTimeNS:      1000,
		ControlTotalWallNS:                  1_106_829,
		CandidateTotalWallNS:                1_020_656,
		ParentWorkload:                      "TinyLlama-1.1B Q4_K_M 200-token decode",
		ParentControlWallNS:                 7_336_289_041,
		ParentCandidateWallNS:               7_352_295_125,
		ParentThroughputRatio:               7_336_289_041.0 / 7_352_295_125,
		LeverageFraction:                    86173.0 / 7_336_289_041,
		MaterialityThreshold:                0.001,
		ParentPromotionGate:                 1.03,
		ExactOutputDigestPassed:             false,
		ControlAllocationBytes:              8,
		CandidateAllocationBytes:            8,
		MixedComputeBlitLifetimePassed:      false,
		ProfilingRecorderCompatible:         false,
		ParentGateRun:                       false,
		Classification:                      "host-only",
		FinalDecision:                       "removed",
	}
	_ = evidence
	runMetalHostOnlyRecorderParentLeverage()
}

func TestMetalHostOnlyRecorderParentLeverageStable(t *testing.T) {
	evidence := recorderLeverageEvidence{
		Hardware:                            "Apple M2 Pro",
		WorkloadShape:                       "32 custom compute encoders",
		WarmColdState:                       "warm",
		ControlCommandBufferOwnershipMode:   "retained references",
		CandidateCommandBufferOwnershipMode: "unretained references",
		LeafWorkload:                        "host-only command construction",
		EncodersPerCommand:                  32,
		LeafControlEncodeNS:                 106829,
		LeafCandidateEncodeNS:               20656,
		LeafSavedEncodeNS:                   86173,
		LeafSpeedup:                         106829.0 / 20656,
		ControlGPUCommandDurationNS:         1_000_000,
		CandidateGPUCommandDurationNS:       1_000_000,
		ControlSynchronizationTimeNS:        1000,
		CandidateSynchronizationTimeNS:      1000,
		ControlTotalWallNS:                  1_106_829,
		CandidateTotalWallNS:                1_020_656,
		ParentWorkload:                      "TinyLlama-1.1B Q4_K_M 200-token decode",
		ParentControlWallNS:                 7_336_289_041,
		ParentCandidateWallNS:               7_352_295_125,
		ParentThroughputRatio:               7_336_289_041.0 / 7_352_295_125,
		LeverageFraction:                    86173.0 / 7_336_289_041,
		MaterialityThreshold:                0.001,
		ParentPromotionGate:                 1.03,
		ExactOutputDigestPassed:             true,
		ControlAllocationBytes:              8,
		CandidateAllocationBytes:            8,
		MixedComputeBlitLifetimePassed:      true,
		ProfilingRecorderCompatible:         true,
		ParentGateRun:                       true,
		Classification:                      "host-only",
		FinalDecision:                       "removed",
	}
	_ = evidence
	runMetalHostOnlyRecorderParentLeverage()
}
