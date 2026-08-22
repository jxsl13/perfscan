package ps6041

import "testing"

type parentKernelInflationReport struct {
	Hardware                        string
	SeamWorkloadIdentity            string
	WarmWeightsResident             bool
	CommandBufferCount              int
	IterationCount                  int
	DispatchesRemoved               int
	DependencyEdgesRemoved          int
	ParentKernelControlNS           float64
	ParentKernelFusedNS             float64
	ParentKernelInflationNS         float64
	RemovedLaunchDependencyCostNS   float64
	ParentInflationBelowRemovedCost bool
	ExtraSpecialFunctions           int
	ExtraReadsBytes                 int
	ControlSeamTotalNS              float64
	CandidateSeamTotalNS            float64
	SeamControlCandidateRatio       float64
	MixedSignOddTailExact           bool
	RepeatedExecutionIdentical      bool
	ControlAllocationBytes          int
	CandidateAllocationBytes        int
	ControlAllocationCount          int
	CandidateAllocationCount        int
	ClaimScope                      string
	FinalDecision                   string
}

func runMetalGPUFusionParentInflationSeamCost() {}

func TestMetalGPUFusionParentInflationSeamCostMissing(t *testing.T) { // want `dispatch-eliminating GPU fusion has no parent-inflation manifest`
	runMetalGPUFusionParentInflationSeamCost()
}

func TestMetalGPUFusionParentInflationSeamCostIncomplete(t *testing.T) {
	evidence := parentKernelInflationReport{ // want `GPU fusion inflation evidence is incomplete; missing seam/workload identity, warm residency status, command-buffer count`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUFusionParentInflationSeamCost()
}

func TestMetalGPUFusionParentInflationSeamCostOverclaim(t *testing.T) {
	evidence := parentKernelInflationReport{ // want `inflation-below-removed-cost status is true but 37181 ns inflation versus 32850 ns removed cost computes false; final decision "retained" retains a fusion whose parent inflation is not recovered by seam savings; claim scope "end-to-end win" reports an end-to-end win despite unrecovered parent inflation`
		Hardware:                        "Apple M2 Pro",
		SeamWorkloadIdentity:            "two Q4_K K2048,N5632 projections plus SwiGLU",
		WarmWeightsResident:             true,
		CommandBufferCount:              1,
		IterationCount:                  200,
		DispatchesRemoved:               1,
		DependencyEdgesRemoved:          1,
		ParentKernelControlNS:           400_000,
		ParentKernelFusedNS:             437_181,
		ParentKernelInflationNS:         37_181,
		RemovedLaunchDependencyCostNS:   32_850,
		ParentInflationBelowRemovedCost: true,
		ExtraSpecialFunctions:           1,
		ExtraReadsBytes:                 22_528,
		ControlSeamTotalNS:              432_850,
		CandidateSeamTotalNS:            437_181,
		SeamControlCandidateRatio:       432_850.0 / 437_181,
		MixedSignOddTailExact:           true,
		RepeatedExecutionIdentical:      true,
		ControlAllocationBytes:          8,
		CandidateAllocationBytes:        8,
		ControlAllocationCount:          1,
		CandidateAllocationCount:        1,
		ClaimScope:                      "end-to-end win",
		FinalDecision:                   "retained",
	}
	_ = evidence
	runMetalGPUFusionParentInflationSeamCost()
}

func TestMetalGPUFusionParentInflationSeamCostStaleArithmetic(t *testing.T) {
	evidence := parentKernelInflationReport{ // want `parent-kernel inflation 1 ns disagrees with fused-control 37181 ns; seam ratio 1.2x disagrees with control/candidate 0.990093x`
		Hardware:                        "Apple M2 Pro",
		SeamWorkloadIdentity:            "two Q4_K K2048,N5632 projections plus SwiGLU",
		WarmWeightsResident:             true,
		CommandBufferCount:              1,
		IterationCount:                  200,
		DispatchesRemoved:               1,
		DependencyEdgesRemoved:          1,
		ParentKernelControlNS:           400_000,
		ParentKernelFusedNS:             437_181,
		ParentKernelInflationNS:         1,
		RemovedLaunchDependencyCostNS:   32_850,
		ParentInflationBelowRemovedCost: false,
		ExtraSpecialFunctions:           1,
		ExtraReadsBytes:                 22_528,
		ControlSeamTotalNS:              432_850,
		CandidateSeamTotalNS:            437_181,
		SeamControlCandidateRatio:       1.2,
		MixedSignOddTailExact:           true,
		RepeatedExecutionIdentical:      true,
		ControlAllocationBytes:          8,
		CandidateAllocationBytes:        8,
		ControlAllocationCount:          1,
		CandidateAllocationCount:        1,
		ClaimScope:                      "end-to-end regression",
		FinalDecision:                   "removed",
	}
	_ = evidence
	runMetalGPUFusionParentInflationSeamCost()
}

func TestMetalGPUFusionParentInflationSeamCostFalseStatus(t *testing.T) {
	evidence := parentKernelInflationReport{ // want `warm residency is explicitly false; mixed-sign odd-tail exactness is explicitly false; repeated-execution parity is explicitly false`
		Hardware:                        "Apple M2 Pro",
		SeamWorkloadIdentity:            "two Q4_K K2048,N5632 projections plus SwiGLU",
		WarmWeightsResident:             false,
		CommandBufferCount:              1,
		IterationCount:                  200,
		DispatchesRemoved:               1,
		DependencyEdgesRemoved:          1,
		ParentKernelControlNS:           400_000,
		ParentKernelFusedNS:             437_181,
		ParentKernelInflationNS:         37_181,
		RemovedLaunchDependencyCostNS:   32_850,
		ParentInflationBelowRemovedCost: false,
		ExtraSpecialFunctions:           1,
		ExtraReadsBytes:                 22_528,
		ControlSeamTotalNS:              432_850,
		CandidateSeamTotalNS:            437_181,
		SeamControlCandidateRatio:       432_850.0 / 437_181,
		MixedSignOddTailExact:           false,
		RepeatedExecutionIdentical:      false,
		ControlAllocationBytes:          8,
		CandidateAllocationBytes:        8,
		ControlAllocationCount:          1,
		CandidateAllocationCount:        1,
		ClaimScope:                      "end-to-end regression",
		FinalDecision:                   "removed",
	}
	_ = evidence
	runMetalGPUFusionParentInflationSeamCost()
}

func TestMetalGPUFusionParentInflationSeamCostStable(t *testing.T) {
	evidence := parentKernelInflationReport{
		Hardware:                        "Apple M2 Pro",
		SeamWorkloadIdentity:            "two Q4_K K2048,N5632 projections plus SwiGLU",
		WarmWeightsResident:             true,
		CommandBufferCount:              1,
		IterationCount:                  200,
		DispatchesRemoved:               1,
		DependencyEdgesRemoved:          1,
		ParentKernelControlNS:           400_000,
		ParentKernelFusedNS:             437_181,
		ParentKernelInflationNS:         37_181,
		RemovedLaunchDependencyCostNS:   32_850,
		ParentInflationBelowRemovedCost: false,
		ExtraSpecialFunctions:           1,
		ExtraReadsBytes:                 22_528,
		ControlSeamTotalNS:              432_850,
		CandidateSeamTotalNS:            437_181,
		SeamControlCandidateRatio:       432_850.0 / 437_181,
		MixedSignOddTailExact:           true,
		RepeatedExecutionIdentical:      true,
		ControlAllocationBytes:          8,
		CandidateAllocationBytes:        8,
		ControlAllocationCount:          1,
		CandidateAllocationCount:        1,
		ClaimScope:                      "end-to-end regression",
		FinalDecision:                   "removed",
	}
	_ = evidence
	runMetalGPUFusionParentInflationSeamCost()
}
