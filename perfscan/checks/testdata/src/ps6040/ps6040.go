package ps6040

import "testing"

type fusionPromotionReport struct {
	Hardware                        string
	WorkloadIdentity                string
	ControlLatencyNS                float64
	CandidateLatencyNS              float64
	MeasuredSpeedup                 float64
	AbsoluteLatencyDeltaNS          float64
	RelativeLatencyReduction        float64
	SampleCount                     int
	ConfidencePassed                bool
	PromotionThreshold              float64
	PromotionVerdict                string
	ControlDispatchCount            int
	CandidateDispatchCount          int
	DispatchDelta                   int
	ControlIntermediateBytes        int
	CandidateIntermediateBytes      int
	EstimatedIntermediateBytesDelta int
	ExactOutputPassed               bool
	ControlAllocationBytes          int
	CandidateAllocationBytes        int
	MeasuredDirection               string
	OutcomeClassification           string
	Retained                        bool
}

func runMetalGPUFusionPromotionReportAbsoluteGain() {}

func TestMetalGPUFusionPromotionReportAbsoluteGainMissing(t *testing.T) { // want `GPU fusion promotion report has no absolute-gain manifest`
	runMetalGPUFusionPromotionReportAbsoluteGain()
}

func TestMetalGPUFusionPromotionReportAbsoluteGainIncomplete(t *testing.T) {
	evidence := fusionPromotionReport{ // want `GPU fusion outcome evidence is incomplete; missing workload identity, control latency, candidate latency`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUFusionPromotionReportAbsoluteGain()
}

func TestMetalGPUFusionPromotionReportAbsoluteGainConflated(t *testing.T) {
	evidence := fusionPromotionReport{ // want `promotion verdict "pass" is not fail for 1.07733x below 1.1x threshold; candidate is retained despite 1.07733x missing 1.1x promotion threshold; measured direction "regression" erases a positive 1.07733x result; outcome classification "rolledback" does not preserve the positive sub-threshold result`
		Hardware:                        "Apple M2 Pro",
		WorkloadIdentity:                "rows=1 residual add plus RMSNorm",
		ControlLatencyNS:                215_615,
		CandidateLatencyNS:              200_139,
		MeasuredSpeedup:                 215_615.0 / 200_139,
		AbsoluteLatencyDeltaNS:          15_476,
		RelativeLatencyReduction:        15_476.0 / 215_615,
		SampleCount:                     200,
		ConfidencePassed:                true,
		PromotionThreshold:              1.10,
		PromotionVerdict:                "pass",
		ControlDispatchCount:            2,
		CandidateDispatchCount:          1,
		DispatchDelta:                   -1,
		ControlIntermediateBytes:        16_384,
		CandidateIntermediateBytes:      0,
		EstimatedIntermediateBytesDelta: -16_384,
		ExactOutputPassed:               true,
		ControlAllocationBytes:          8,
		CandidateAllocationBytes:        8,
		MeasuredDirection:               "regression",
		OutcomeClassification:           "rolled-back",
		Retained:                        true,
	}
	_ = evidence
	runMetalGPUFusionPromotionReportAbsoluteGain()
}

func TestMetalGPUFusionPromotionReportAbsoluteGainStaleArithmetic(t *testing.T) {
	evidence := fusionPromotionReport{ // want `measured speedup 1.2x disagrees with latency ratio 1.07733x; absolute latency delta 1 ns disagrees with control-candidate 15476 ns; relative latency reduction 0.5 disagrees with measured 0.0717761; dispatch delta 1 disagrees with candidate-control -1; intermediate-bytes delta 1 disagrees with candidate-control -16384`
		Hardware:                        "Apple M2 Pro",
		WorkloadIdentity:                "rows=1 residual add plus RMSNorm",
		ControlLatencyNS:                215_615,
		CandidateLatencyNS:              200_139,
		MeasuredSpeedup:                 1.2,
		AbsoluteLatencyDeltaNS:          1,
		RelativeLatencyReduction:        0.5,
		SampleCount:                     200,
		ConfidencePassed:                true,
		PromotionThreshold:              1.10,
		PromotionVerdict:                "pass",
		ControlDispatchCount:            2,
		CandidateDispatchCount:          1,
		DispatchDelta:                   1,
		ControlIntermediateBytes:        16_384,
		CandidateIntermediateBytes:      0,
		EstimatedIntermediateBytesDelta: 1,
		ExactOutputPassed:               true,
		ControlAllocationBytes:          8,
		CandidateAllocationBytes:        8,
		MeasuredDirection:               "improvement",
		OutcomeClassification:           "positive",
		Retained:                        true,
	}
	_ = evidence
	runMetalGPUFusionPromotionReportAbsoluteGain()
}

func TestMetalGPUFusionPromotionReportAbsoluteGainFalseQuality(t *testing.T) {
	evidence := fusionPromotionReport{ // want `confidence/sample quality is explicitly false; exact-output parity is explicitly false; sample count must be positive`
		Hardware:                        "Apple M2 Pro",
		WorkloadIdentity:                "rows=1 residual add plus RMSNorm",
		ControlLatencyNS:                215_615,
		CandidateLatencyNS:              200_139,
		MeasuredSpeedup:                 215_615.0 / 200_139,
		AbsoluteLatencyDeltaNS:          15_476,
		RelativeLatencyReduction:        15_476.0 / 215_615,
		SampleCount:                     0,
		ConfidencePassed:                false,
		PromotionThreshold:              1.10,
		PromotionVerdict:                "fail",
		ControlDispatchCount:            2,
		CandidateDispatchCount:          1,
		DispatchDelta:                   -1,
		ControlIntermediateBytes:        16_384,
		CandidateIntermediateBytes:      0,
		EstimatedIntermediateBytesDelta: -16_384,
		ExactOutputPassed:               false,
		ControlAllocationBytes:          8,
		CandidateAllocationBytes:        8,
		MeasuredDirection:               "improvement",
		OutcomeClassification:           "positive-below-threshold",
		Retained:                        false,
	}
	_ = evidence
	runMetalGPUFusionPromotionReportAbsoluteGain()
}

func TestMetalGPUFusionPromotionReportAbsoluteGainStable(t *testing.T) {
	evidence := fusionPromotionReport{
		Hardware:                        "Apple M2 Pro",
		WorkloadIdentity:                "rows=1 residual add plus RMSNorm",
		ControlLatencyNS:                215_615,
		CandidateLatencyNS:              200_139,
		MeasuredSpeedup:                 215_615.0 / 200_139,
		AbsoluteLatencyDeltaNS:          15_476,
		RelativeLatencyReduction:        15_476.0 / 215_615,
		SampleCount:                     200,
		ConfidencePassed:                true,
		PromotionThreshold:              1.10,
		PromotionVerdict:                "fail",
		ControlDispatchCount:            2,
		CandidateDispatchCount:          1,
		DispatchDelta:                   -1,
		ControlIntermediateBytes:        16_384,
		CandidateIntermediateBytes:      0,
		EstimatedIntermediateBytesDelta: -16_384,
		ExactOutputPassed:               true,
		ControlAllocationBytes:          8,
		CandidateAllocationBytes:        8,
		MeasuredDirection:               "improvement",
		OutcomeClassification:           "positive-below-threshold",
		Retained:                        false,
	}
	_ = evidence
	runMetalGPUFusionPromotionReportAbsoluteGain()
}
