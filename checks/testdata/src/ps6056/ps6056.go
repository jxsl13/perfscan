package ps6056

import "testing"

type groupedFusionPromotionEvidence struct {
	Hardware                            string
	WorkloadIdentity                    string
	CandidateDefaultOff                 bool
	ShortScreenProcessCount             int
	ShortScreenIterationCount           int
	ShortScreenControlNS                float64
	ShortScreenCandidateNS              float64
	ShortScreenControlCandidateRatio    float64
	ShortScreenUsedForPromotion         bool
	GroupedProcessCount                 int
	GroupedSampleCountPerSide           int
	GroupedControlRawSamplesNS          []float64
	GroupedCandidateRawSamplesNS        []float64
	GroupedExecutionOrder               []string
	GroupedControlMedianNS              float64
	GroupedCandidateMedianNS            float64
	GroupedControlCandidateRatio        float64
	GroupedScreenUsedForPromotion       bool
	PromotionContractPredeclared        bool
	PromotionThresholdPredeclared       bool
	RequiredIndependentPairCount        int
	AcceptedIndependentProcessPairCount int
	FreshProcessesVerified              bool
	OrderAlternatingVerified            bool
	PromotionIterationsPerProcess       int
	PromotionWarmupDisclosure           string
	FixedSynchronizationBoundaries      bool
	PromotionPairOrder                  []string
	PromotionControlRawSamplesNS        []float64
	PromotionCandidateRawSamplesNS      []float64
	RawSamplesPublished                 bool
	PromotionControlMedianNS            float64
	PromotionCandidateMedianNS          float64
	PromotionControlCandidateRatio      float64
	ControlAllocationBytes              int
	CandidateAllocationBytes            int
	ControlAllocationCount              int
	CandidateAllocationCount            int
	ExactOutputParity                   bool
	PromotionThreshold                  float64
	PromotionVerdict                    string
	CandidateSelected                   bool
	EvidenceClassification              string
	FinalDecision                       string
}

func runMetalGPUGroupedShortScreenAlternatingPromotionContract() {}

func TestMetalGPUGroupedShortScreenAlternatingPromotionContractMissing(t *testing.T) { // want `grouped GPU fusion screen has no alternating promotion manifest`
	runMetalGPUGroupedShortScreenAlternatingPromotionContract()
}

func TestMetalGPUGroupedShortScreenAlternatingPromotionContractIncomplete(t *testing.T) {
	evidence := groupedFusionPromotionEvidence{ // want `grouped GPU screen evidence is incomplete; missing workload identity, candidate default-off status, short-screen process count`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUGroupedShortScreenAlternatingPromotionContract()
}

func TestMetalGPUGroupedShortScreenAlternatingPromotionContractOverclaim(t *testing.T) {
	evidence := groupedFusionPromotionEvidence{ // want `promotion verdict "pass" disagrees with computed independent gate; single-process short screen is used directly for promotion; blocked same-process grouped screen is used directly for promotion; fusion candidate is selected without a passing independent alternating promotion gate; final decision "retained" retains a candidate below the independent promotion gate`
		Hardware:                            "Apple M2 Pro",
		WorkloadIdentity:                    "Q6_K matmul residual-accumulate fusion M1,K2048,N5632",
		CandidateDefaultOff:                 true,
		ShortScreenProcessCount:             1,
		ShortScreenIterationCount:           200,
		ShortScreenControlNS:                1_259_000,
		ShortScreenCandidateNS:              1_000_000,
		ShortScreenControlCandidateRatio:    1.259,
		ShortScreenUsedForPromotion:         true,
		GroupedProcessCount:                 1,
		GroupedSampleCountPerSide:           3,
		GroupedControlRawSamplesNS:          []float64{408000, 410000, 412000},
		GroupedCandidateRawSamplesNS:        []float64{248000, 250000, 252000},
		GroupedExecutionOrder:               []string{"control", "control", "control", "candidate", "candidate", "candidate"},
		GroupedControlMedianNS:              410000,
		GroupedCandidateMedianNS:            250000,
		GroupedControlCandidateRatio:        1.640,
		GroupedScreenUsedForPromotion:       true,
		PromotionContractPredeclared:        true,
		PromotionThresholdPredeclared:       true,
		RequiredIndependentPairCount:        10,
		AcceptedIndependentProcessPairCount: 10,
		FreshProcessesVerified:              true,
		OrderAlternatingVerified:            true,
		PromotionIterationsPerProcess:       500,
		PromotionWarmupDisclosure:           "20 local warmups per implementation before each timed 500x loop",
		FixedSynchronizationBoundaries:      true,
		PromotionPairOrder:                  []string{"control-candidate", "candidate-control", "control-candidate", "candidate-control", "control-candidate", "candidate-control", "control-candidate", "candidate-control", "control-candidate", "candidate-control"},
		PromotionControlRawSamplesNS:        []float64{365000, 368000, 369000, 370000, 370977, 370978, 372000, 374000, 376000, 378000},
		PromotionCandidateRawSamplesNS:      []float64{352000, 354000, 356000, 358000, 359694, 359696, 361000, 363000, 365000, 367000},
		RawSamplesPublished:                 true,
		PromotionControlMedianNS:            370977.5,
		PromotionCandidateMedianNS:          359695,
		PromotionControlCandidateRatio:      370977.5 / 359695,
		ControlAllocationBytes:              8,
		CandidateAllocationBytes:            8,
		ControlAllocationCount:              1,
		CandidateAllocationCount:            1,
		ExactOutputParity:                   true,
		PromotionThreshold:                  1.10,
		PromotionVerdict:                    "pass",
		CandidateSelected:                   true,
		EvidenceClassification:              "grouped-screen-win",
		FinalDecision:                       "retained",
	}
	_ = evidence
	runMetalGPUGroupedShortScreenAlternatingPromotionContract()
}

func TestMetalGPUGroupedShortScreenAlternatingPromotionContractStale(t *testing.T) {
	evidence := groupedFusionPromotionEvidence{ // want `short-screen ratio 2x disagrees with control/candidate 1.259x; grouped control/candidate ratio 2x disagrees with 1.64x; promotion control/candidate ratio 2x disagrees with 1.03137x`
		Hardware:                            "Apple M2 Pro",
		WorkloadIdentity:                    "Q6_K matmul residual-accumulate fusion M1,K2048,N5632",
		CandidateDefaultOff:                 true,
		ShortScreenProcessCount:             1,
		ShortScreenIterationCount:           200,
		ShortScreenControlNS:                1_259_000,
		ShortScreenCandidateNS:              1_000_000,
		ShortScreenControlCandidateRatio:    2,
		ShortScreenUsedForPromotion:         false,
		GroupedProcessCount:                 1,
		GroupedSampleCountPerSide:           3,
		GroupedControlRawSamplesNS:          []float64{408000, 410000, 412000},
		GroupedCandidateRawSamplesNS:        []float64{248000, 250000, 252000},
		GroupedExecutionOrder:               []string{"control", "control", "control", "candidate", "candidate", "candidate"},
		GroupedControlMedianNS:              410000,
		GroupedCandidateMedianNS:            250000,
		GroupedControlCandidateRatio:        2,
		GroupedScreenUsedForPromotion:       false,
		PromotionContractPredeclared:        true,
		PromotionThresholdPredeclared:       true,
		RequiredIndependentPairCount:        10,
		AcceptedIndependentProcessPairCount: 10,
		FreshProcessesVerified:              true,
		OrderAlternatingVerified:            true,
		PromotionIterationsPerProcess:       500,
		PromotionWarmupDisclosure:           "20 local warmups per implementation before each timed 500x loop",
		FixedSynchronizationBoundaries:      true,
		PromotionPairOrder:                  []string{"control-candidate", "candidate-control", "control-candidate", "candidate-control", "control-candidate", "candidate-control", "control-candidate", "candidate-control", "control-candidate", "candidate-control"},
		PromotionControlRawSamplesNS:        []float64{365000, 368000, 369000, 370000, 370977, 370978, 372000, 374000, 376000, 378000},
		PromotionCandidateRawSamplesNS:      []float64{352000, 354000, 356000, 358000, 359694, 359696, 361000, 363000, 365000, 367000},
		RawSamplesPublished:                 true,
		PromotionControlMedianNS:            370977.5,
		PromotionCandidateMedianNS:          359695,
		PromotionControlCandidateRatio:      2,
		ControlAllocationBytes:              8,
		CandidateAllocationBytes:            8,
		ControlAllocationCount:              1,
		CandidateAllocationCount:            1,
		ExactOutputParity:                   true,
		PromotionThreshold:                  1.10,
		PromotionVerdict:                    "fail",
		CandidateSelected:                   false,
		EvidenceClassification:              "grouped-screen-refuted",
		FinalDecision:                       "removed",
	}
	_ = evidence
	runMetalGPUGroupedShortScreenAlternatingPromotionContract()
}

func TestMetalGPUGroupedShortScreenAlternatingPromotionContractStable(t *testing.T) {
	evidence := groupedFusionPromotionEvidence{
		Hardware:                            "Apple M2 Pro",
		WorkloadIdentity:                    "Q6_K matmul residual-accumulate fusion M1,K2048,N5632",
		CandidateDefaultOff:                 true,
		ShortScreenProcessCount:             1,
		ShortScreenIterationCount:           200,
		ShortScreenControlNS:                1_259_000,
		ShortScreenCandidateNS:              1_000_000,
		ShortScreenControlCandidateRatio:    1.259,
		ShortScreenUsedForPromotion:         false,
		GroupedProcessCount:                 1,
		GroupedSampleCountPerSide:           3,
		GroupedControlRawSamplesNS:          []float64{408000, 410000, 412000},
		GroupedCandidateRawSamplesNS:        []float64{248000, 250000, 252000},
		GroupedExecutionOrder:               []string{"control", "control", "control", "candidate", "candidate", "candidate"},
		GroupedControlMedianNS:              410000,
		GroupedCandidateMedianNS:            250000,
		GroupedControlCandidateRatio:        1.640,
		GroupedScreenUsedForPromotion:       false,
		PromotionContractPredeclared:        true,
		PromotionThresholdPredeclared:       true,
		RequiredIndependentPairCount:        10,
		AcceptedIndependentProcessPairCount: 10,
		FreshProcessesVerified:              true,
		OrderAlternatingVerified:            true,
		PromotionIterationsPerProcess:       500,
		PromotionWarmupDisclosure:           "20 local warmups per implementation before each timed 500x loop",
		FixedSynchronizationBoundaries:      true,
		PromotionPairOrder:                  []string{"control-candidate", "candidate-control", "control-candidate", "candidate-control", "control-candidate", "candidate-control", "control-candidate", "candidate-control", "control-candidate", "candidate-control"},
		PromotionControlRawSamplesNS:        []float64{365000, 368000, 369000, 370000, 370977, 370978, 372000, 374000, 376000, 378000},
		PromotionCandidateRawSamplesNS:      []float64{352000, 354000, 356000, 358000, 359694, 359696, 361000, 363000, 365000, 367000},
		RawSamplesPublished:                 true,
		PromotionControlMedianNS:            370977.5,
		PromotionCandidateMedianNS:          359695,
		PromotionControlCandidateRatio:      370977.5 / 359695,
		ControlAllocationBytes:              8,
		CandidateAllocationBytes:            8,
		ControlAllocationCount:              1,
		CandidateAllocationCount:            1,
		ExactOutputParity:                   true,
		PromotionThreshold:                  1.10,
		PromotionVerdict:                    "fail",
		CandidateSelected:                   false,
		EvidenceClassification:              "grouped-screen-refuted",
		FinalDecision:                       "removed",
	}
	_ = evidence
	runMetalGPUGroupedShortScreenAlternatingPromotionContract()
}
