package ps6033

import "testing"

type commandConstructionEvidence struct {
	Hardware                                         string
	OptimizationIdentity                             string
	ImmutableArgumentBytes                           int
	LeafWorkload                                     string
	DispatchesPerCommand                             int
	LeafControlRecordingNS                           float64
	LeafCandidateRecordingNS                         float64
	LeafRecordingRatio                               float64
	LeafControlGPUNS                                 float64
	LeafCandidateGPUNS                               float64
	LeafGPURatio                                     float64
	LeafControlWallNS                                float64
	LeafCandidateWallNS                              float64
	LeafWallRatio                                    float64
	EvidenceClassification                           string
	ParentWorkload                                   string
	ParentControlNSToken                             float64
	ParentCandidateNSToken                           float64
	ParentMarginalRatio                              float64
	ParentPairedRatio                                float64
	ParentPromotionThreshold                         float64
	LeafFreshProcessPairs                            int
	ParentFreshProcessPairs                          int
	AlternatingOrderPassed                           bool
	CPUGPUOverlapConsidered                          bool
	ControlAllocationBytes                           int
	CandidateAllocationBytes                         int
	LeafExactDigestPassed                            bool
	ParentExactDigestPassed                          bool
	CloseAfterEncodingBeforeCompletionLifetimePassed bool
	FinalDecision                                    string
}

func runMetalCachedArgumentBufferRecordingParentLeverage() {}

func TestMetalCachedArgumentBufferRecordingParentLeverageMissing(t *testing.T) { // want `cached command-construction campaign has no host/parent leverage manifest`
	runMetalCachedArgumentBufferRecordingParentLeverage()
}

func TestMetalCachedArgumentBufferRecordingParentLeverageIncomplete(t *testing.T) {
	evidence := commandConstructionEvidence{ // want `command-construction leverage evidence is incomplete; missing optimization identity, immutable argument bytes, leaf workload`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalCachedArgumentBufferRecordingParentLeverage()
}

func TestMetalCachedArgumentBufferRecordingParentLeverageOverclaimed(t *testing.T) {
	evidence := commandConstructionEvidence{ // want `evidence classification "application-leverage" overstates a host-bound leaf while the parent misses 1.03x; final decision "retained" retains a candidate whose parent marginal/paired ratio misses 1.03x`
		Hardware:                 "Apple M2 Pro",
		OptimizationIdentity:     "cached immutable M=1,K,N argument buffer",
		ImmutableArgumentBytes:   12,
		LeafWorkload:             "Q4_K K2048,N2048 1024 dispatches",
		DispatchesPerCommand:     1024,
		LeafControlRecordingNS:   4680,
		LeafCandidateRecordingNS: 520,
		LeafRecordingRatio:       4680.0 / 520,
		LeafControlGPUNS:         10000,
		LeafCandidateGPUNS:       9900,
		LeafGPURatio:             10000.0 / 9900,
		LeafControlWallNS:        20627,
		LeafCandidateWallNS:      15843,
		LeafWallRatio:            20627.0 / 15843,
		EvidenceClassification:   "application-leverage",
		ParentWorkload:           "TinyLlama-1.1B Q4_K_M 200-token decode",
		ParentControlNSToken:     36_797_829.5,
		ParentCandidateNSToken:   36_764_181,
		ParentMarginalRatio:      36_797_829.5 / 36_764_181,
		ParentPairedRatio:        1.000613,
		ParentPromotionThreshold: 1.03,
		LeafFreshProcessPairs:    5,
		ParentFreshProcessPairs:  10,
		AlternatingOrderPassed:   true,
		CPUGPUOverlapConsidered:  true,
		ControlAllocationBytes:   147760,
		CandidateAllocationBytes: 147760,
		LeafExactDigestPassed:    true,
		ParentExactDigestPassed:  true,
		CloseAfterEncodingBeforeCompletionLifetimePassed: true,
		FinalDecision: "retained",
	}
	_ = evidence
	runMetalCachedArgumentBufferRecordingParentLeverage()
}

func TestMetalCachedArgumentBufferRecordingParentLeverageRatioMismatch(t *testing.T) {
	evidence := commandConstructionEvidence{ // want `leaf recording ratio 8x disagrees with control/candidate timing ratio 9x`
		Hardware:                 "Apple M2 Pro",
		OptimizationIdentity:     "cached immutable M=1,K,N argument buffer",
		ImmutableArgumentBytes:   12,
		LeafWorkload:             "Q4_K K2048,N2048 1024 dispatches",
		DispatchesPerCommand:     1024,
		LeafControlRecordingNS:   4680,
		LeafCandidateRecordingNS: 520,
		LeafRecordingRatio:       8,
		LeafControlGPUNS:         10000,
		LeafCandidateGPUNS:       9900,
		LeafGPURatio:             10000.0 / 9900,
		LeafControlWallNS:        20627,
		LeafCandidateWallNS:      15843,
		LeafWallRatio:            20627.0 / 15843,
		EvidenceClassification:   "host-bound-leaf",
		ParentWorkload:           "TinyLlama-1.1B Q4_K_M 200-token decode",
		ParentControlNSToken:     36_797_829.5,
		ParentCandidateNSToken:   36_764_181,
		ParentMarginalRatio:      36_797_829.5 / 36_764_181,
		ParentPairedRatio:        1.000613,
		ParentPromotionThreshold: 1.03,
		LeafFreshProcessPairs:    5,
		ParentFreshProcessPairs:  10,
		AlternatingOrderPassed:   true,
		CPUGPUOverlapConsidered:  true,
		ControlAllocationBytes:   147760,
		CandidateAllocationBytes: 147760,
		LeafExactDigestPassed:    true,
		ParentExactDigestPassed:  true,
		CloseAfterEncodingBeforeCompletionLifetimePassed: true,
		FinalDecision: "removed",
	}
	_ = evidence
	runMetalCachedArgumentBufferRecordingParentLeverage()
}

func TestMetalCachedArgumentBufferRecordingParentLeverageFalseGates(t *testing.T) {
	evidence := commandConstructionEvidence{ // want `CPU/GPU overlap consideration is explicitly false; parent exact digest is explicitly false; close-before-completion lifetime is explicitly false`
		Hardware:                 "Apple M2 Pro",
		OptimizationIdentity:     "cached immutable M=1,K,N argument buffer",
		ImmutableArgumentBytes:   12,
		LeafWorkload:             "Q4_K K2048,N2048 1024 dispatches",
		DispatchesPerCommand:     1024,
		LeafControlRecordingNS:   4680,
		LeafCandidateRecordingNS: 520,
		LeafRecordingRatio:       4680.0 / 520,
		LeafControlGPUNS:         10000,
		LeafCandidateGPUNS:       9900,
		LeafGPURatio:             10000.0 / 9900,
		LeafControlWallNS:        20627,
		LeafCandidateWallNS:      15843,
		LeafWallRatio:            20627.0 / 15843,
		EvidenceClassification:   "host-bound-leaf",
		ParentWorkload:           "TinyLlama-1.1B Q4_K_M 200-token decode",
		ParentControlNSToken:     36_797_829.5,
		ParentCandidateNSToken:   36_764_181,
		ParentMarginalRatio:      36_797_829.5 / 36_764_181,
		ParentPairedRatio:        1.000613,
		ParentPromotionThreshold: 1.03,
		LeafFreshProcessPairs:    5,
		ParentFreshProcessPairs:  10,
		AlternatingOrderPassed:   true,
		CPUGPUOverlapConsidered:  false,
		ControlAllocationBytes:   147760,
		CandidateAllocationBytes: 147760,
		LeafExactDigestPassed:    true,
		ParentExactDigestPassed:  false,
		CloseAfterEncodingBeforeCompletionLifetimePassed: false,
		FinalDecision: "removed",
	}
	_ = evidence
	runMetalCachedArgumentBufferRecordingParentLeverage()
}

func TestMetalCachedArgumentBufferRecordingParentLeverageStable(t *testing.T) {
	evidence := commandConstructionEvidence{
		Hardware:                 "Apple M2 Pro",
		OptimizationIdentity:     "cached immutable M=1,K,N argument buffer",
		ImmutableArgumentBytes:   12,
		LeafWorkload:             "Q4_K K2048,N2048 1024 dispatches",
		DispatchesPerCommand:     1024,
		LeafControlRecordingNS:   4680,
		LeafCandidateRecordingNS: 520,
		LeafRecordingRatio:       4680.0 / 520,
		LeafControlGPUNS:         10000,
		LeafCandidateGPUNS:       9900,
		LeafGPURatio:             10000.0 / 9900,
		LeafControlWallNS:        20627,
		LeafCandidateWallNS:      15843,
		LeafWallRatio:            20627.0 / 15843,
		EvidenceClassification:   "host-bound-leaf",
		ParentWorkload:           "TinyLlama-1.1B Q4_K_M 200-token decode",
		ParentControlNSToken:     36_797_829.5,
		ParentCandidateNSToken:   36_764_181,
		ParentMarginalRatio:      36_797_829.5 / 36_764_181,
		ParentPairedRatio:        1.000613,
		ParentPromotionThreshold: 1.03,
		LeafFreshProcessPairs:    5,
		ParentFreshProcessPairs:  10,
		AlternatingOrderPassed:   true,
		CPUGPUOverlapConsidered:  true,
		ControlAllocationBytes:   147760,
		CandidateAllocationBytes: 147760,
		LeafExactDigestPassed:    true,
		ParentExactDigestPassed:  true,
		CloseAfterEncodingBeforeCompletionLifetimePassed: true,
		FinalDecision: "removed",
	}
	_ = evidence
	runMetalCachedArgumentBufferRecordingParentLeverage()
}
