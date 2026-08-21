package ps6042

import "testing"

type gpuRegionBarrierReport struct {
	Hardware                             string
	OSIdentity                           string
	ModelWorkloadIdentity                string
	SynchronizationPattern               string
	WarmWeightsResident                  bool
	IterationCount                       int
	ControlCommandBuffersPerSubmission   int
	CandidateCommandBuffersPerSubmission int
	ControlComputeRegions                int
	CandidateComputeRegions              int
	ControlNonComputeEncoders            int
	CandidateNonComputeEncoders          int
	ControlEncoderEvents                 int
	CandidateEncoderEvents               int
	EncoderReductionRatio                float64
	ControlDispatches                    int
	CandidateDispatches                  int
	CandidateDispatchesPerRegion         float64
	ControlExplicitBarriers              int
	CandidateExplicitBarriers            int
	CandidateBarriersPerDispatch         float64
	ControlLongestDependencyDepth        int
	CandidateLongestDependencyDepth      int
	DependencyDepthControlCandidateRatio float64
	ControlEndToEndNS                    float64
	CandidateEndToEndNS                  float64
	EndToEndControlCandidateRatio        float64
	PromotionThreshold                   float64
	PromotionVerdict                     string
	ExactTwoStepLogits                   bool
	Classification                       string
	FinalDecision                        string
}

func runMetalGPUEncoderCollapseBarrierDepth() {}

func TestMetalGPUEncoderCollapseBarrierDepthMissing(t *testing.T) { // want `GPU encoder-collapse campaign has no barrier-depth manifest`
	runMetalGPUEncoderCollapseBarrierDepth()
}

func TestMetalGPUEncoderCollapseBarrierDepthIncomplete(t *testing.T) {
	evidence := gpuRegionBarrierReport{ // want `GPU encoder-collapse evidence is incomplete; missing OS identity, model/workload identity, synchronization pattern`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUEncoderCollapseBarrierDepth()
}

func TestMetalGPUEncoderCollapseBarrierDepthOverclaim(t *testing.T) {
	evidence := gpuRegionBarrierReport{ // want `classification "performance-improvement" turns a 7.55556x structural collapse with near-unchanged dispatch/depth into a performance win; final decision "retained" retains an encoder collapse below the end-to-end promotion gate`
		Hardware:                             "Apple M2 Pro",
		OSIdentity:                           "macOS 26.5.1",
		ModelWorkloadIdentity:                "TinyLlama 1.1B Q4_K_M one-token decode",
		SynchronizationPattern:               "one token per synchronized command-buffer submission",
		WarmWeightsResident:                  true,
		IterationCount:                       200,
		ControlCommandBuffersPerSubmission:   1,
		CandidateCommandBuffersPerSubmission: 1,
		ControlComputeRegions:                340,
		CandidateComputeRegions:              23,
		ControlNonComputeEncoders:            0,
		CandidateNonComputeEncoders:          22,
		ControlEncoderEvents:                 340,
		CandidateEncoderEvents:               45,
		EncoderReductionRatio:                340.0 / 45,
		ControlDispatches:                    296,
		CandidateDispatches:                  296,
		CandidateDispatchesPerRegion:         296.0 / 23,
		ControlExplicitBarriers:              221,
		CandidateExplicitBarriers:            221,
		CandidateBarriersPerDispatch:         221.0 / 296,
		ControlLongestDependencyDepth:        222,
		CandidateLongestDependencyDepth:      222,
		DependencyDepthControlCandidateRatio: 1,
		ControlEndToEndNS:                    7_376_051_833,
		CandidateEndToEndNS:                  7_344_744_834,
		EndToEndControlCandidateRatio:        7_376_051_833.0 / 7_344_744_834,
		PromotionThreshold:                   1.10,
		PromotionVerdict:                     "fail",
		ExactTwoStepLogits:                   true,
		Classification:                       "performance-improvement",
		FinalDecision:                        "retained",
	}
	_ = evidence
	runMetalGPUEncoderCollapseBarrierDepth()
}

func TestMetalGPUEncoderCollapseBarrierDepthStaleArithmetic(t *testing.T) {
	evidence := gpuRegionBarrierReport{ // want `candidate regions\+non-compute encoders 45 disagree with 44 encoder events; encoder reduction ratio 7x disagrees with control/candidate 7.72727x; candidate dispatches/region 1 disagrees with 12.8696; candidate barriers/dispatch 1 disagrees with 0.746622; dependency-depth ratio 2x disagrees with control/candidate 1x; end-to-end ratio 1.2x disagrees with control/candidate 1.00426x`
		Hardware:                             "Apple M2 Pro",
		OSIdentity:                           "macOS 26.5.1",
		ModelWorkloadIdentity:                "TinyLlama 1.1B Q4_K_M one-token decode",
		SynchronizationPattern:               "one token per synchronized command-buffer submission",
		WarmWeightsResident:                  true,
		IterationCount:                       200,
		ControlCommandBuffersPerSubmission:   1,
		CandidateCommandBuffersPerSubmission: 1,
		ControlComputeRegions:                340,
		CandidateComputeRegions:              23,
		ControlNonComputeEncoders:            0,
		CandidateNonComputeEncoders:          22,
		ControlEncoderEvents:                 340,
		CandidateEncoderEvents:               44,
		EncoderReductionRatio:                7,
		ControlDispatches:                    296,
		CandidateDispatches:                  296,
		CandidateDispatchesPerRegion:         1,
		ControlExplicitBarriers:              221,
		CandidateExplicitBarriers:            221,
		CandidateBarriersPerDispatch:         1,
		ControlLongestDependencyDepth:        222,
		CandidateLongestDependencyDepth:      222,
		DependencyDepthControlCandidateRatio: 2,
		ControlEndToEndNS:                    7_376_051_833,
		CandidateEndToEndNS:                  7_344_744_834,
		EndToEndControlCandidateRatio:        1.2,
		PromotionThreshold:                   1.10,
		PromotionVerdict:                     "fail",
		ExactTwoStepLogits:                   true,
		Classification:                       "structural-only",
		FinalDecision:                        "removed",
	}
	_ = evidence
	runMetalGPUEncoderCollapseBarrierDepth()
}

func TestMetalGPUEncoderCollapseBarrierDepthFalseStatus(t *testing.T) {
	evidence := gpuRegionBarrierReport{ // want `warm residency is explicitly false; exact output is explicitly false`
		Hardware:                             "Apple M2 Pro",
		OSIdentity:                           "macOS 26.5.1",
		ModelWorkloadIdentity:                "TinyLlama 1.1B Q4_K_M one-token decode",
		SynchronizationPattern:               "one token per synchronized command-buffer submission",
		WarmWeightsResident:                  false,
		IterationCount:                       200,
		ControlCommandBuffersPerSubmission:   1,
		CandidateCommandBuffersPerSubmission: 1,
		ControlComputeRegions:                340,
		CandidateComputeRegions:              23,
		ControlNonComputeEncoders:            0,
		CandidateNonComputeEncoders:          22,
		ControlEncoderEvents:                 340,
		CandidateEncoderEvents:               45,
		EncoderReductionRatio:                340.0 / 45,
		ControlDispatches:                    296,
		CandidateDispatches:                  296,
		CandidateDispatchesPerRegion:         296.0 / 23,
		ControlExplicitBarriers:              221,
		CandidateExplicitBarriers:            221,
		CandidateBarriersPerDispatch:         221.0 / 296,
		ControlLongestDependencyDepth:        222,
		CandidateLongestDependencyDepth:      222,
		DependencyDepthControlCandidateRatio: 1,
		ControlEndToEndNS:                    7_376_051_833,
		CandidateEndToEndNS:                  7_344_744_834,
		EndToEndControlCandidateRatio:        7_376_051_833.0 / 7_344_744_834,
		PromotionThreshold:                   1.10,
		PromotionVerdict:                     "fail",
		ExactTwoStepLogits:                   false,
		Classification:                       "structural-only",
		FinalDecision:                        "removed",
	}
	_ = evidence
	runMetalGPUEncoderCollapseBarrierDepth()
}

func TestMetalGPUEncoderCollapseBarrierDepthStable(t *testing.T) {
	evidence := gpuRegionBarrierReport{
		Hardware:                             "Apple M2 Pro",
		OSIdentity:                           "macOS 26.5.1",
		ModelWorkloadIdentity:                "TinyLlama 1.1B Q4_K_M one-token decode",
		SynchronizationPattern:               "one token per synchronized command-buffer submission",
		WarmWeightsResident:                  true,
		IterationCount:                       200,
		ControlCommandBuffersPerSubmission:   1,
		CandidateCommandBuffersPerSubmission: 1,
		ControlComputeRegions:                340,
		CandidateComputeRegions:              23,
		ControlNonComputeEncoders:            0,
		CandidateNonComputeEncoders:          22,
		ControlEncoderEvents:                 340,
		CandidateEncoderEvents:               45,
		EncoderReductionRatio:                340.0 / 45,
		ControlDispatches:                    296,
		CandidateDispatches:                  296,
		CandidateDispatchesPerRegion:         296.0 / 23,
		ControlExplicitBarriers:              221,
		CandidateExplicitBarriers:            221,
		CandidateBarriersPerDispatch:         221.0 / 296,
		ControlLongestDependencyDepth:        222,
		CandidateLongestDependencyDepth:      222,
		DependencyDepthControlCandidateRatio: 1,
		ControlEndToEndNS:                    7_376_051_833,
		CandidateEndToEndNS:                  7_344_744_834,
		EndToEndControlCandidateRatio:        7_376_051_833.0 / 7_344_744_834,
		PromotionThreshold:                   1.10,
		PromotionVerdict:                     "fail",
		ExactTwoStepLogits:                   true,
		Classification:                       "structural-only",
		FinalDecision:                        "removed",
	}
	_ = evidence
	runMetalGPUEncoderCollapseBarrierDepth()
}
