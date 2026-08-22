package ps6055

import "testing"

type metalFusionRankingEvidence struct {
	Hardware                               string
	ToolchainIdentity                      string
	SeamShapeIdentity                      string
	CandidateDefaultOff                    bool
	WarmInputsResident                     bool
	ProductionQ4KPipelineUnchanged         bool
	ProductionSwiGLUPipelineUnchanged      bool
	ControlCommandBufferCount              int
	CandidateCommandBufferCount            int
	ControlComputeEncoderCount             int
	CandidateComputeEncoderCount           int
	EncoderReductionRatio                  float64
	ControlDispatchCount                   int
	CandidateDispatchCount                 int
	CandidateExplicitBarrierCount          int
	CPUEncodingSubmissionPressureMeasured  bool
	ControlCPUEncodingSubmissionNS         float64
	CandidateCPUEncodingSubmissionNS       float64
	TrafficReductionDocumented             bool
	ControlIntermediateTrafficBytes        int
	CandidateIntermediateTrafficBytes      int
	SynchronizationReductionDocumented     bool
	ControlSynchronizationEventCount       int
	CandidateSynchronizationEventCount     int
	StageBoundaryIntervalInflationExcluded bool
	CompleteOneCommandBufferSeam           bool
	FixedIterationCount                    int
	GroupedProfilerLabelCount              int
	ControlSeamNS                          float64
	CandidateSeamNS                        float64
	SeamControlCandidateRatio              float64
	ControlAllocationBytes                 int
	CandidateAllocationBytes               int
	ControlAllocationCount                 int
	CandidateAllocationCount               int
	ExactOddTailParity                     bool
	PerformanceMinimum                     float64
	PerformanceVerdict                     string
	CandidateRanked                        bool
	RemovedBeforePairedAndModelStages      bool
	Classification                         string
	FinalDecision                          string
}

func runMetalEncoderCountFusionRankingOneCommandBufferSeam() {}

func TestMetalEncoderCountFusionRankingOneCommandBufferSeamMissing(t *testing.T) { // want `Metal encoder-count fusion ranking has no priced seam manifest`
	runMetalEncoderCountFusionRankingOneCommandBufferSeam()
}

func TestMetalEncoderCountFusionRankingOneCommandBufferSeamIncomplete(t *testing.T) {
	evidence := metalFusionRankingEvidence{ // want `Metal fusion-ranking evidence is incomplete; missing toolchain identity, seam shape identity, candidate default-off status`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalEncoderCountFusionRankingOneCommandBufferSeam()
}

func TestMetalEncoderCountFusionRankingOneCommandBufferSeamOverrank(t *testing.T) {
	evidence := metalFusionRankingEvidence{ // want `performance verdict "pass" disagrees with computed seam gate; fusion candidate is ranked from encoder count despite absent priced leverage or a failed complete-seam performance gate; final decision "retained" retains an encoder-count candidate without priced leverage and a passing seam`
		Hardware:                               "Apple M2 Pro",
		ToolchainIdentity:                      "Xcode 26.6 Metal",
		SeamShapeIdentity:                      "M1,K2048,N5632 Q4_K matvec + binary SwiGLU",
		CandidateDefaultOff:                    true,
		WarmInputsResident:                     true,
		ProductionQ4KPipelineUnchanged:         true,
		ProductionSwiGLUPipelineUnchanged:      true,
		ControlCommandBufferCount:              1,
		CandidateCommandBufferCount:            1,
		ControlComputeEncoderCount:             3,
		CandidateComputeEncoderCount:           1,
		EncoderReductionRatio:                  3,
		ControlDispatchCount:                   3,
		CandidateDispatchCount:                 3,
		CandidateExplicitBarrierCount:          1,
		CPUEncodingSubmissionPressureMeasured:  false,
		ControlCPUEncodingSubmissionNS:         0,
		CandidateCPUEncodingSubmissionNS:       0,
		TrafficReductionDocumented:             false,
		ControlIntermediateTrafficBytes:        45056,
		CandidateIntermediateTrafficBytes:      45056,
		SynchronizationReductionDocumented:     false,
		ControlSynchronizationEventCount:       1,
		CandidateSynchronizationEventCount:     1,
		StageBoundaryIntervalInflationExcluded: true,
		CompleteOneCommandBufferSeam:           true,
		FixedIterationCount:                    200,
		GroupedProfilerLabelCount:              1,
		ControlSeamNS:                          426352,
		CandidateSeamNS:                        425334,
		SeamControlCandidateRatio:              426352.0 / 425334,
		ControlAllocationBytes:                 8,
		CandidateAllocationBytes:               8,
		ControlAllocationCount:                 1,
		CandidateAllocationCount:               1,
		ExactOddTailParity:                     true,
		PerformanceMinimum:                     1.01,
		PerformanceVerdict:                     "pass",
		CandidateRanked:                        true,
		RemovedBeforePairedAndModelStages:      false,
		Classification:                         "encoder-count-win",
		FinalDecision:                          "retained",
	}
	_ = evidence
	runMetalEncoderCountFusionRankingOneCommandBufferSeam()
}

func TestMetalEncoderCountFusionRankingOneCommandBufferSeamStale(t *testing.T) {
	evidence := metalFusionRankingEvidence{ // want `encoder reduction ratio 4x disagrees with control/candidate 3x; traffic-reduction claim is true but control/candidate bytes are 45056/45056; synchronization-reduction claim is true but control/candidate event counts are 1/1; seam control/candidate ratio 2x disagrees with 1.00239x`
		Hardware:                               "Apple M2 Pro",
		ToolchainIdentity:                      "Xcode 26.6 Metal",
		SeamShapeIdentity:                      "M1,K2048,N5632 Q4_K matvec + binary SwiGLU",
		CandidateDefaultOff:                    true,
		WarmInputsResident:                     true,
		ProductionQ4KPipelineUnchanged:         true,
		ProductionSwiGLUPipelineUnchanged:      true,
		ControlCommandBufferCount:              1,
		CandidateCommandBufferCount:            1,
		ControlComputeEncoderCount:             3,
		CandidateComputeEncoderCount:           1,
		EncoderReductionRatio:                  4,
		ControlDispatchCount:                   3,
		CandidateDispatchCount:                 3,
		CandidateExplicitBarrierCount:          1,
		CPUEncodingSubmissionPressureMeasured:  false,
		ControlCPUEncodingSubmissionNS:         0,
		CandidateCPUEncodingSubmissionNS:       0,
		TrafficReductionDocumented:             true,
		ControlIntermediateTrafficBytes:        45056,
		CandidateIntermediateTrafficBytes:      45056,
		SynchronizationReductionDocumented:     true,
		ControlSynchronizationEventCount:       1,
		CandidateSynchronizationEventCount:     1,
		StageBoundaryIntervalInflationExcluded: true,
		CompleteOneCommandBufferSeam:           true,
		FixedIterationCount:                    200,
		GroupedProfilerLabelCount:              1,
		ControlSeamNS:                          426352,
		CandidateSeamNS:                        425334,
		SeamControlCandidateRatio:              2,
		ControlAllocationBytes:                 8,
		CandidateAllocationBytes:               8,
		ControlAllocationCount:                 1,
		CandidateAllocationCount:               1,
		ExactOddTailParity:                     true,
		PerformanceMinimum:                     1.01,
		PerformanceVerdict:                     "fail",
		CandidateRanked:                        false,
		RemovedBeforePairedAndModelStages:      true,
		Classification:                         "negligible-seam-result",
		FinalDecision:                          "removed",
	}
	_ = evidence
	runMetalEncoderCountFusionRankingOneCommandBufferSeam()
}

func TestMetalEncoderCountFusionRankingOneCommandBufferSeamStable(t *testing.T) {
	evidence := metalFusionRankingEvidence{
		Hardware:                               "Apple M2 Pro",
		ToolchainIdentity:                      "Xcode 26.6 Metal",
		SeamShapeIdentity:                      "M1,K2048,N5632 Q4_K matvec + binary SwiGLU",
		CandidateDefaultOff:                    true,
		WarmInputsResident:                     true,
		ProductionQ4KPipelineUnchanged:         true,
		ProductionSwiGLUPipelineUnchanged:      true,
		ControlCommandBufferCount:              1,
		CandidateCommandBufferCount:            1,
		ControlComputeEncoderCount:             3,
		CandidateComputeEncoderCount:           1,
		EncoderReductionRatio:                  3,
		ControlDispatchCount:                   3,
		CandidateDispatchCount:                 3,
		CandidateExplicitBarrierCount:          1,
		CPUEncodingSubmissionPressureMeasured:  false,
		ControlCPUEncodingSubmissionNS:         0,
		CandidateCPUEncodingSubmissionNS:       0,
		TrafficReductionDocumented:             false,
		ControlIntermediateTrafficBytes:        45056,
		CandidateIntermediateTrafficBytes:      45056,
		SynchronizationReductionDocumented:     false,
		ControlSynchronizationEventCount:       1,
		CandidateSynchronizationEventCount:     1,
		StageBoundaryIntervalInflationExcluded: true,
		CompleteOneCommandBufferSeam:           true,
		FixedIterationCount:                    200,
		GroupedProfilerLabelCount:              1,
		ControlSeamNS:                          426352,
		CandidateSeamNS:                        425334,
		SeamControlCandidateRatio:              426352.0 / 425334,
		ControlAllocationBytes:                 8,
		CandidateAllocationBytes:               8,
		ControlAllocationCount:                 1,
		CandidateAllocationCount:               1,
		ExactOddTailParity:                     true,
		PerformanceMinimum:                     1.01,
		PerformanceVerdict:                     "fail",
		CandidateRanked:                        false,
		RemovedBeforePairedAndModelStages:      true,
		Classification:                         "negligible-seam-result",
		FinalDecision:                          "removed",
	}
	_ = evidence
	runMetalEncoderCountFusionRankingOneCommandBufferSeam()
}
