package ps6047

import "testing"

type packedLoadExperiment struct {
	Hardware                          string
	OSIdentity                        string
	GoVersion                         string
	ToolchainComponentBuild           string
	NativeTarget                      string
	QuantFormat                       string
	RowStrideBytes                    int
	MinimumRowAlignmentBytes          int
	SelectedDeviceLoadWidthBytes      int
	AssembledWordWidthBytes           int
	Direct32BitLoadsValid             bool
	LoadSchedule                      string
	ControlPipelineIdentity           string
	CandidatePipelineIdentity         string
	CandidatePipelineSelected         bool
	FallbackMeasurementExcluded       bool
	MixedSignOddTailParity            bool
	IncidentShapes                    []string
	IncidentControlLatenciesNS        []float64
	IncidentCandidateLatenciesNS      []float64
	IncidentSpeedups                  []float64
	IncidentControlAllocationBytes    []float64
	IncidentCandidateAllocationBytes  []float64
	IncidentControlAllocationCounts   []float64
	IncidentCandidateAllocationCounts []float64
	MinimumIncidentSpeedup            float64
	AllIncidentShapesImproved         bool
	SameAIRLibraryProvenance          bool
	ControlNativeTextBytes            int
	CandidateNativeTextBytes          int
	NativeTextDeltaBytes              int
	NativeTextReductionFraction       float64
	ControlSpillSectionBytes          int
	CandidateSpillSectionBytes        int
	GPUStatsMetadataEmpty             bool
	RegisterCountsKnown               bool
	PromotionThreshold                float64
	PromotionVerdict                  string
	Classification                    string
	FinalDecision                     string
}

func runMetalQ6KQuantUnpackPackedLoadSchedule() {}

func TestMetalQ6KQuantUnpackPackedLoadScheduleMissing(t *testing.T) { // want `packed quant-unpack campaign has no alignment/compiler manifest`
	runMetalQ6KQuantUnpackPackedLoadSchedule()
}

func TestMetalQ6KQuantUnpackPackedLoadScheduleIncomplete(t *testing.T) {
	evidence := packedLoadExperiment{ // want `packed quant-unpack evidence is incomplete; missing OS identity, Go version, toolchain build`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalQ6KQuantUnpackPackedLoadSchedule()
}

func TestMetalQ6KQuantUnpackPackedLoadScheduleOverclaim(t *testing.T) {
	evidence := packedLoadExperiment{ // want `promotion verdict "pass" disagrees with minimum 1.01178x versus 1.1x threshold; final decision "retained" retains a packed-load candidate below the keep threshold`
		Hardware:                          "Apple M2 Pro 19-core GPU",
		OSIdentity:                        "macOS 26.5.1",
		GoVersion:                         "Go 1.26.6",
		ToolchainComponentBuild:           "Metal Toolchain 17F109",
		NativeTarget:                      "applegpu_g14s",
		QuantFormat:                       "Q6_K",
		RowStrideBytes:                    210,
		MinimumRowAlignmentBytes:          2,
		SelectedDeviceLoadWidthBytes:      2,
		AssembledWordWidthBytes:           4,
		Direct32BitLoadsValid:             false,
		LoadSchedule:                      "two adjacent ushort loads combined into uint",
		ControlPipelineIdentity:           "q6k scalar-byte cooperative pipeline",
		CandidatePipelineIdentity:         "q6k packed-ushort cooperative pipeline",
		CandidatePipelineSelected:         true,
		FallbackMeasurementExcluded:       true,
		MixedSignOddTailParity:            true,
		IncidentShapes:                    []string{"K2048,N256", "K2048,N2048", "K5632,N2048", "K2048,N32000"},
		IncidentControlLatenciesNS:        []float64{201684, 238035, 216020, 453078},
		IncidentCandidateLatenciesNS:      []float64{199336, 230667, 210106, 436945},
		IncidentSpeedups:                  []float64{201684.0 / 199336, 238035.0 / 230667, 216020.0 / 210106, 453078.0 / 436945},
		IncidentControlAllocationBytes:    []float64{8, 8, 8, 8},
		IncidentCandidateAllocationBytes:  []float64{8, 8, 8, 8},
		IncidentControlAllocationCounts:   []float64{1, 1, 1, 1},
		IncidentCandidateAllocationCounts: []float64{1, 1, 1, 1},
		MinimumIncidentSpeedup:            201684.0 / 199336,
		AllIncidentShapesImproved:         true,
		SameAIRLibraryProvenance:          true,
		ControlNativeTextBytes:            2738,
		CandidateNativeTextBytes:          2440,
		NativeTextDeltaBytes:              -298,
		NativeTextReductionFraction:       298.0 / 2738,
		ControlSpillSectionBytes:          0,
		CandidateSpillSectionBytes:        0,
		GPUStatsMetadataEmpty:             true,
		RegisterCountsKnown:               false,
		PromotionThreshold:                1.10,
		PromotionVerdict:                  "pass",
		Classification:                    "compiler-code-size-win-leaf-subthreshold",
		FinalDecision:                     "retained",
	}
	_ = evidence
	runMetalQ6KQuantUnpackPackedLoadSchedule()
}

func TestMetalQ6KQuantUnpackPackedLoadScheduleStale(t *testing.T) {
	evidence := packedLoadExperiment{ // want `incident speedup for "K2048,N256" is 2x, want 1.01178x`
		Hardware:                          "Apple M2 Pro 19-core GPU",
		OSIdentity:                        "macOS 26.5.1",
		GoVersion:                         "Go 1.26.6",
		ToolchainComponentBuild:           "Metal Toolchain 17F109",
		NativeTarget:                      "applegpu_g14s",
		QuantFormat:                       "Q6_K",
		RowStrideBytes:                    210,
		MinimumRowAlignmentBytes:          2,
		SelectedDeviceLoadWidthBytes:      2,
		AssembledWordWidthBytes:           4,
		Direct32BitLoadsValid:             false,
		LoadSchedule:                      "two adjacent ushort loads combined into uint",
		ControlPipelineIdentity:           "q6k scalar-byte cooperative pipeline",
		CandidatePipelineIdentity:         "q6k packed-ushort cooperative pipeline",
		CandidatePipelineSelected:         true,
		FallbackMeasurementExcluded:       true,
		MixedSignOddTailParity:            true,
		IncidentShapes:                    []string{"K2048,N256", "K2048,N2048", "K5632,N2048", "K2048,N32000"},
		IncidentControlLatenciesNS:        []float64{201684, 238035, 216020, 453078},
		IncidentCandidateLatenciesNS:      []float64{199336, 230667, 210106, 436945},
		IncidentSpeedups:                  []float64{2, 2, 2, 2},
		IncidentControlAllocationBytes:    []float64{8, 8, 8, 8},
		IncidentCandidateAllocationBytes:  []float64{8, 8, 8, 8},
		IncidentControlAllocationCounts:   []float64{1, 1, 1, 1},
		IncidentCandidateAllocationCounts: []float64{1, 1, 1, 1},
		MinimumIncidentSpeedup:            2,
		AllIncidentShapesImproved:         true,
		SameAIRLibraryProvenance:          true,
		ControlNativeTextBytes:            2738,
		CandidateNativeTextBytes:          2440,
		NativeTextDeltaBytes:              1,
		NativeTextReductionFraction:       1,
		ControlSpillSectionBytes:          0,
		CandidateSpillSectionBytes:        0,
		GPUStatsMetadataEmpty:             true,
		RegisterCountsKnown:               false,
		PromotionThreshold:                1.10,
		PromotionVerdict:                  "pass",
		Classification:                    "stale",
		FinalDecision:                     "removed",
	}
	_ = evidence
	runMetalQ6KQuantUnpackPackedLoadSchedule()
}

func TestMetalQ6KQuantUnpackPackedLoadScheduleStable(t *testing.T) {
	evidence := packedLoadExperiment{
		Hardware:                          "Apple M2 Pro 19-core GPU",
		OSIdentity:                        "macOS 26.5.1",
		GoVersion:                         "Go 1.26.6",
		ToolchainComponentBuild:           "Metal Toolchain 17F109",
		NativeTarget:                      "applegpu_g14s",
		QuantFormat:                       "Q6_K",
		RowStrideBytes:                    210,
		MinimumRowAlignmentBytes:          2,
		SelectedDeviceLoadWidthBytes:      2,
		AssembledWordWidthBytes:           4,
		Direct32BitLoadsValid:             false,
		LoadSchedule:                      "two adjacent ushort loads combined into uint",
		ControlPipelineIdentity:           "q6k scalar-byte cooperative pipeline",
		CandidatePipelineIdentity:         "q6k packed-ushort cooperative pipeline",
		CandidatePipelineSelected:         true,
		FallbackMeasurementExcluded:       true,
		MixedSignOddTailParity:            true,
		IncidentShapes:                    []string{"K2048,N256", "K2048,N2048", "K5632,N2048", "K2048,N32000"},
		IncidentControlLatenciesNS:        []float64{201684, 238035, 216020, 453078},
		IncidentCandidateLatenciesNS:      []float64{199336, 230667, 210106, 436945},
		IncidentSpeedups:                  []float64{201684.0 / 199336, 238035.0 / 230667, 216020.0 / 210106, 453078.0 / 436945},
		IncidentControlAllocationBytes:    []float64{8, 8, 8, 8},
		IncidentCandidateAllocationBytes:  []float64{8, 8, 8, 8},
		IncidentControlAllocationCounts:   []float64{1, 1, 1, 1},
		IncidentCandidateAllocationCounts: []float64{1, 1, 1, 1},
		MinimumIncidentSpeedup:            201684.0 / 199336,
		AllIncidentShapesImproved:         true,
		SameAIRLibraryProvenance:          true,
		ControlNativeTextBytes:            2738,
		CandidateNativeTextBytes:          2440,
		NativeTextDeltaBytes:              -298,
		NativeTextReductionFraction:       298.0 / 2738,
		ControlSpillSectionBytes:          0,
		CandidateSpillSectionBytes:        0,
		GPUStatsMetadataEmpty:             true,
		RegisterCountsKnown:               false,
		PromotionThreshold:                1.10,
		PromotionVerdict:                  "fail",
		Classification:                    "compiler-code-size-win-leaf-subthreshold",
		FinalDecision:                     "removed",
	}
	_ = evidence
	runMetalQ6KQuantUnpackPackedLoadSchedule()
}
