package ps6044

import "testing"

type independentPromotionEvidence struct {
	Hardware                    string
	WorkloadIdentity            string
	ScreenSameProcess           bool
	ScreenFixedOrder            bool
	LocalWarmupCompleted        bool
	ScreenIterationCount        int
	ScreenControlNS             float64
	ScreenGroupedNS             float64
	ScreenCandidateNS           float64
	ScreenControlCandidateRatio float64
	ControlAllocationBytes      int
	CandidateAllocationBytes    int
	ControlAllocationCount      int
	CandidateAllocationCount    int
	IndependentProcessCount     int
	PromotionIterationCount     int
	FreshProcessValidation      bool
	AlternatingOrderPassed      bool
	ControlProcessLatenciesNS   []float64
	CandidateProcessLatenciesNS []float64
	PairedRatios                []float64
	ControlMedianNS             float64
	CandidateMedianNS           float64
	RatioOfMedians              float64
	MedianPairedRatio           float64
	ControlP90NS                float64
	CandidateP90NS              float64
	ScreenPromotionDivergence   float64
	ExactTwoStepLogits          bool
	PromotionThreshold          float64
	PromotionVerdict            string
	EvidenceClassification      string
	FinalDecision               string
}

func runMetalGPUScreenIndependentPromotionAlternating() {}

func TestMetalGPUScreenIndependentPromotionAlternatingMissing(t *testing.T) { // want `same-process GPU screen has no independent alternating promotion manifest`
	runMetalGPUScreenIndependentPromotionAlternating()
}

func TestMetalGPUScreenIndependentPromotionAlternatingIncomplete(t *testing.T) {
	evidence := independentPromotionEvidence{ // want `GPU screen/promotion evidence is incomplete; missing workload identity, same-process screen status, fixed-order screen status`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUScreenIndependentPromotionAlternating()
}

func TestMetalGPUScreenIndependentPromotionAlternatingOverclaim(t *testing.T) {
	evidence := independentPromotionEvidence{ // want `promotion verdict "pass" disagrees with independent 1.00144x versus 1.01x threshold; evidence classification "screen-win" does not mark the large screen win as refuted; final decision "retained" retains a candidate whose same-process screen disappeared independently`
		Hardware:                    "Apple M2 Pro",
		WorkloadIdentity:            "two independent Q4_K M=1 projections plus SwiGLU",
		ScreenSameProcess:           true,
		ScreenFixedOrder:            true,
		LocalWarmupCompleted:        true,
		ScreenIterationCount:        200,
		ScreenControlNS:             428_713,
		ScreenGroupedNS:             455_646,
		ScreenCandidateNS:           307_358,
		ScreenControlCandidateRatio: 428_713.0 / 307_358,
		ControlAllocationBytes:      8,
		CandidateAllocationBytes:    8,
		ControlAllocationCount:      1,
		CandidateAllocationCount:    1,
		IndependentProcessCount:     10,
		PromotionIterationCount:     500,
		FreshProcessValidation:      true,
		AlternatingOrderPassed:      true,
		ControlProcessLatenciesNS:   []float64{426000, 427000, 428000, 428500, 429000, 429222, 430000, 430500, 431198, 432000},
		CandidateProcessLatenciesNS: []float64{425000, 426500, 427500, 428000, 428400, 428588, 429000, 430000, 434737, 435000},
		PairedRatios: []float64{
			426000.0 / 425000, 427000.0 / 426500, 428000.0 / 427500, 428500.0 / 428000, 429000.0 / 428400,
			429222.0 / 428588, 430000.0 / 429000, 430500.0 / 430000, 431198.0 / 434737, 432000.0 / 435000,
		},
		ControlMedianNS:           429_111,
		CandidateMedianNS:         428_494,
		RatioOfMedians:            429_111.0 / 428_494,
		MedianPairedRatio:         1.0011709617929152,
		ControlP90NS:              431_198,
		CandidateP90NS:            434_737,
		ScreenPromotionDivergence: (428_713.0 / 307_358) / (429_111.0 / 428_494),
		ExactTwoStepLogits:        true,
		PromotionThreshold:        1.01,
		PromotionVerdict:          "pass",
		EvidenceClassification:    "screen-win",
		FinalDecision:             "retained",
	}
	_ = evidence
	runMetalGPUScreenIndependentPromotionAlternating()
}

func TestMetalGPUScreenIndependentPromotionAlternatingStale(t *testing.T) {
	evidence := independentPromotionEvidence{ // want `screen ratio 1.5x disagrees with control/candidate 1.39483x`
		Hardware:                    "Apple M2 Pro",
		WorkloadIdentity:            "two independent Q4_K M=1 projections plus SwiGLU",
		ScreenSameProcess:           true,
		ScreenFixedOrder:            true,
		LocalWarmupCompleted:        true,
		ScreenIterationCount:        200,
		ScreenControlNS:             428_713,
		ScreenGroupedNS:             455_646,
		ScreenCandidateNS:           307_358,
		ScreenControlCandidateRatio: 1.5,
		ControlAllocationBytes:      8,
		CandidateAllocationBytes:    8,
		ControlAllocationCount:      1,
		CandidateAllocationCount:    1,
		IndependentProcessCount:     10,
		PromotionIterationCount:     500,
		FreshProcessValidation:      true,
		AlternatingOrderPassed:      true,
		ControlProcessLatenciesNS:   []float64{426000, 427000, 428000, 428500, 429000, 429222, 430000, 430500, 431198, 432000},
		CandidateProcessLatenciesNS: []float64{425000, 426500, 427500, 428000, 428400, 428588, 429000, 430000, 434737, 435000},
		PairedRatios:                []float64{1.2, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		ControlMedianNS:             1,
		CandidateMedianNS:           1,
		RatioOfMedians:              1,
		MedianPairedRatio:           1,
		ControlP90NS:                1,
		CandidateP90NS:              1,
		ScreenPromotionDivergence:   1,
		ExactTwoStepLogits:          true,
		PromotionThreshold:          1.01,
		PromotionVerdict:            "fail",
		EvidenceClassification:      "screen-refuted",
		FinalDecision:               "removed",
	}
	_ = evidence
	runMetalGPUScreenIndependentPromotionAlternating()
}

func TestMetalGPUScreenIndependentPromotionAlternatingStable(t *testing.T) {
	evidence := independentPromotionEvidence{
		Hardware:                    "Apple M2 Pro",
		WorkloadIdentity:            "two independent Q4_K M=1 projections plus SwiGLU",
		ScreenSameProcess:           true,
		ScreenFixedOrder:            true,
		LocalWarmupCompleted:        true,
		ScreenIterationCount:        200,
		ScreenControlNS:             428_713,
		ScreenGroupedNS:             455_646,
		ScreenCandidateNS:           307_358,
		ScreenControlCandidateRatio: 428_713.0 / 307_358,
		ControlAllocationBytes:      8,
		CandidateAllocationBytes:    8,
		ControlAllocationCount:      1,
		CandidateAllocationCount:    1,
		IndependentProcessCount:     10,
		PromotionIterationCount:     500,
		FreshProcessValidation:      true,
		AlternatingOrderPassed:      true,
		ControlProcessLatenciesNS:   []float64{426000, 427000, 428000, 428500, 429000, 429222, 430000, 430500, 431198, 432000},
		CandidateProcessLatenciesNS: []float64{425000, 426500, 427500, 428000, 428400, 428588, 429000, 430000, 434737, 435000},
		PairedRatios: []float64{
			426000.0 / 425000, 427000.0 / 426500, 428000.0 / 427500, 428500.0 / 428000, 429000.0 / 428400,
			429222.0 / 428588, 430000.0 / 429000, 430500.0 / 430000, 431198.0 / 434737, 432000.0 / 435000,
		},
		ControlMedianNS:           429_111,
		CandidateMedianNS:         428_494,
		RatioOfMedians:            429_111.0 / 428_494,
		MedianPairedRatio:         1.0011709617929152,
		ControlP90NS:              431_198,
		CandidateP90NS:            434_737,
		ScreenPromotionDivergence: (428_713.0 / 307_358) / (429_111.0 / 428_494),
		ExactTwoStepLogits:        true,
		PromotionThreshold:        1.01,
		PromotionVerdict:          "fail",
		EvidenceClassification:    "screen-refuted",
		FinalDecision:             "removed",
	}
	_ = evidence
	runMetalGPUScreenIndependentPromotionAlternating()
}
