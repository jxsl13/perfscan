package ps6046

import "testing"

type serialEncoderValidationReport struct {
	Hardware                        string
	OSIdentity                      string
	GoVersion                       string
	CandidateDefaultOff             bool
	RawSameBinaryControl            bool
	ControlArchitectureMode         string
	CandidateArchitectureMode       string
	ControlCommandBufferCount       int
	CandidateCommandBufferCount     int
	ControlComputeEncoderCount      int
	CandidateComputeEncoderCount    int
	ControlBlitEncoderCount         int
	CandidateBlitEncoderCount       int
	ControlFrameworkEncoderCount    int
	CandidateFrameworkEncoderCount  int
	ControlDispatchCount            int
	CandidateDispatchCount          int
	ControlBarrierCount             int
	CandidateBarrierCount           int
	BoundaryCauses                  []string
	MixedComputeBlitFrameworkParity bool
	PhysicalFinishParity            bool
	CommitWaitParity                bool
	ScreenDepths                    []float64
	ScreenControlTimesNS            []float64
	ScreenCandidateTimesNS          []float64
	ScreenRatios                    []float64
	ScreenIterationCount            int
	ScreenIsTriageOnly              bool
	ValidationDepth                 int
	IndependentProcessCount         int
	ValidationIterationCount        int
	AlternatingOrderPassed          bool
	ValidationControlLatenciesNS    []float64
	ValidationCandidateLatenciesNS  []float64
	ValidationPairedRatios          []float64
	ValidationControlMedianNS       float64
	ValidationCandidateMedianNS     float64
	ValidationRatioOfMedians        float64
	CandidateTailSamplesNS          []float64
	ControlAllocationBytes          int
	CandidateAllocationBytes        int
	ControlAllocationCount          int
	CandidateAllocationCount        int
	PromotionThreshold              float64
	PromotionVerdict                string
	CandidateArchitectureScope      string
	ReferenceArchitectureBundle     []string
	FullModelPromotionAllowed       bool
	Classification                  string
	FinalDecision                   string
}

func runMetalExecutorPersistentSerialEncoderFullGraphValidation() {}

func TestMetalExecutorPersistentSerialEncoderFullGraphValidationMissing(t *testing.T) { // want `persistent Metal encoder campaign has no full-graph validation manifest`
	runMetalExecutorPersistentSerialEncoderFullGraphValidation()
}

func TestMetalExecutorPersistentSerialEncoderFullGraphValidationIncomplete(t *testing.T) {
	evidence := serialEncoderValidationReport{ // want `Metal executor validation evidence is incomplete; missing OS identity, Go version, candidate default-off status`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalExecutorPersistentSerialEncoderFullGraphValidation()
}

func TestMetalExecutorPersistentSerialEncoderFullGraphValidationOverclaim(t *testing.T) {
	evidence := serialEncoderValidationReport{ // want `promotion verdict "pass" disagrees with 1.03695x versus 1.1x gate; full-model promotion is allowed despite 1.03695x missing 1.1x gate; classification "full-graph-executor-speedup" overgeneralizes serial encoder persistence into full-graph executor leverage; final decision "retained" retains a candidate that failed repeated validation`
		Hardware:                        "Apple M2 Pro",
		OSIdentity:                      "macOS 26.5.1",
		GoVersion:                       "Go 1.26.6",
		CandidateDefaultOff:             true,
		RawSameBinaryControl:            true,
		ControlArchitectureMode:         "one compute encoder per custom dispatch",
		CandidateArchitectureMode:       "persistent serial compute encoder segmented at boundaries",
		ControlCommandBufferCount:       1,
		CandidateCommandBufferCount:     1,
		ControlComputeEncoderCount:      340,
		CandidateComputeEncoderCount:    1,
		ControlBlitEncoderCount:         22,
		CandidateBlitEncoderCount:       22,
		ControlFrameworkEncoderCount:    4,
		CandidateFrameworkEncoderCount:  4,
		ControlDispatchCount:            340,
		CandidateDispatchCount:          340,
		ControlBarrierCount:             0,
		CandidateBarrierCount:           0,
		BoundaryCauses:                  []string{"blit encoder", "MPS framework encoder", "command-buffer end"},
		MixedComputeBlitFrameworkParity: true,
		PhysicalFinishParity:            true,
		CommitWaitParity:                true,
		ScreenDepths:                    []float64{32, 340},
		ScreenControlTimesNS:            []float64{547528, 2730736},
		ScreenCandidateTimesNS:          []float64{474480, 2453739},
		ScreenRatios:                    []float64{547528.0 / 474480, 2730736.0 / 2453739},
		ScreenIterationCount:            200,
		ScreenIsTriageOnly:              true,
		ValidationDepth:                 340,
		IndependentProcessCount:         10,
		ValidationIterationCount:        500,
		AlternatingOrderPassed:          true,
		ValidationControlLatenciesNS:    []float64{2700000, 2710000, 2720000, 2730000, 2742800, 2742833, 2760000, 2770000, 2780000, 2790000},
		ValidationCandidateLatenciesNS:  []float64{2600000, 2610000, 2620000, 2630000, 2645000, 2645138, 2660000, 2670000, 3184000, 3230000},
		ValidationPairedRatios: []float64{
			2700000.0 / 2600000, 2710000.0 / 2610000, 2720000.0 / 2620000, 2730000.0 / 2630000, 2742800.0 / 2645000,
			2742833.0 / 2645138, 2760000.0 / 2660000, 2770000.0 / 2670000, 2780000.0 / 3184000, 2790000.0 / 3230000,
		},
		ValidationControlMedianNS:   2_742_816.5,
		ValidationCandidateMedianNS: 2_645_069,
		ValidationRatioOfMedians:    2_742_816.5 / 2_645_069,
		CandidateTailSamplesNS:      []float64{3_184_000, 3_230_000},
		ControlAllocationBytes:      8,
		CandidateAllocationBytes:    8,
		ControlAllocationCount:      1,
		CandidateAllocationCount:    1,
		PromotionThreshold:          1.10,
		PromotionVerdict:            "pass",
		CandidateArchitectureScope:  "serial-encoder-persistence-only",
		ReferenceArchitectureBundle: []string{"dependency-aware concurrent dispatch", "buffer barriers", "graph fusion and optimization", "graph partitioning over multiple command buffers"},
		FullModelPromotionAllowed:   true,
		Classification:              "full-graph-executor-speedup",
		FinalDecision:               "retained",
	}
	_ = evidence
	runMetalExecutorPersistentSerialEncoderFullGraphValidation()
}

func TestMetalExecutorPersistentSerialEncoderFullGraphValidationStaleScreen(t *testing.T) {
	evidence := serialEncoderValidationReport{ // want `screen ratio at depth 32 is 2x, want 1.15395x`
		Hardware:                        "Apple M2 Pro",
		OSIdentity:                      "macOS 26.5.1",
		GoVersion:                       "Go 1.26.6",
		CandidateDefaultOff:             true,
		RawSameBinaryControl:            true,
		ControlArchitectureMode:         "one compute encoder per custom dispatch",
		CandidateArchitectureMode:       "persistent serial compute encoder segmented at boundaries",
		ControlCommandBufferCount:       1,
		CandidateCommandBufferCount:     1,
		ControlComputeEncoderCount:      340,
		CandidateComputeEncoderCount:    1,
		ControlBlitEncoderCount:         22,
		CandidateBlitEncoderCount:       22,
		ControlFrameworkEncoderCount:    4,
		CandidateFrameworkEncoderCount:  4,
		ControlDispatchCount:            340,
		CandidateDispatchCount:          340,
		ControlBarrierCount:             0,
		CandidateBarrierCount:           0,
		BoundaryCauses:                  []string{"blit encoder", "MPS framework encoder", "command-buffer end"},
		MixedComputeBlitFrameworkParity: true,
		PhysicalFinishParity:            true,
		CommitWaitParity:                true,
		ScreenDepths:                    []float64{32, 340},
		ScreenControlTimesNS:            []float64{547528, 2730736},
		ScreenCandidateTimesNS:          []float64{474480, 2453739},
		ScreenRatios:                    []float64{2, 2},
		ScreenIterationCount:            200,
		ScreenIsTriageOnly:              true,
		ValidationDepth:                 340,
		IndependentProcessCount:         10,
		ValidationIterationCount:        500,
		AlternatingOrderPassed:          true,
		ValidationControlLatenciesNS:    []float64{2700000, 2710000, 2720000, 2730000, 2742800, 2742833, 2760000, 2770000, 2780000, 2790000},
		ValidationCandidateLatenciesNS:  []float64{2600000, 2610000, 2620000, 2630000, 2645000, 2645138, 2660000, 2670000, 3184000, 3230000},
		ValidationPairedRatios: []float64{
			2700000.0 / 2600000, 2710000.0 / 2610000, 2720000.0 / 2620000, 2730000.0 / 2630000, 2742800.0 / 2645000,
			2742833.0 / 2645138, 2760000.0 / 2660000, 2770000.0 / 2670000, 2780000.0 / 3184000, 2790000.0 / 3230000,
		},
		ValidationControlMedianNS:   2_742_816.5,
		ValidationCandidateMedianNS: 2_645_069,
		ValidationRatioOfMedians:    2_742_816.5 / 2_645_069,
		CandidateTailSamplesNS:      []float64{3_184_000, 3_230_000},
		ControlAllocationBytes:      8,
		CandidateAllocationBytes:    8,
		ControlAllocationCount:      1,
		CandidateAllocationCount:    1,
		PromotionThreshold:          1.10,
		PromotionVerdict:            "fail",
		CandidateArchitectureScope:  "serial-encoder-persistence-only",
		ReferenceArchitectureBundle: []string{"dependency-aware concurrent dispatch", "buffer barriers", "graph fusion and optimization", "graph partitioning over multiple command buffers"},
		FullModelPromotionAllowed:   false,
		Classification:              "serial-component-only",
		FinalDecision:               "removed",
	}
	_ = evidence
	runMetalExecutorPersistentSerialEncoderFullGraphValidation()
}

func TestMetalExecutorPersistentSerialEncoderFullGraphValidationStable(t *testing.T) {
	evidence := serialEncoderValidationReport{
		Hardware:                        "Apple M2 Pro",
		OSIdentity:                      "macOS 26.5.1",
		GoVersion:                       "Go 1.26.6",
		CandidateDefaultOff:             true,
		RawSameBinaryControl:            true,
		ControlArchitectureMode:         "one compute encoder per custom dispatch",
		CandidateArchitectureMode:       "persistent serial compute encoder segmented at boundaries",
		ControlCommandBufferCount:       1,
		CandidateCommandBufferCount:     1,
		ControlComputeEncoderCount:      340,
		CandidateComputeEncoderCount:    1,
		ControlBlitEncoderCount:         22,
		CandidateBlitEncoderCount:       22,
		ControlFrameworkEncoderCount:    4,
		CandidateFrameworkEncoderCount:  4,
		ControlDispatchCount:            340,
		CandidateDispatchCount:          340,
		ControlBarrierCount:             0,
		CandidateBarrierCount:           0,
		BoundaryCauses:                  []string{"blit encoder", "MPS framework encoder", "command-buffer end"},
		MixedComputeBlitFrameworkParity: true,
		PhysicalFinishParity:            true,
		CommitWaitParity:                true,
		ScreenDepths:                    []float64{32, 340},
		ScreenControlTimesNS:            []float64{547528, 2730736},
		ScreenCandidateTimesNS:          []float64{474480, 2453739},
		ScreenRatios:                    []float64{547528.0 / 474480, 2730736.0 / 2453739},
		ScreenIterationCount:            200,
		ScreenIsTriageOnly:              true,
		ValidationDepth:                 340,
		IndependentProcessCount:         10,
		ValidationIterationCount:        500,
		AlternatingOrderPassed:          true,
		ValidationControlLatenciesNS:    []float64{2700000, 2710000, 2720000, 2730000, 2742800, 2742833, 2760000, 2770000, 2780000, 2790000},
		ValidationCandidateLatenciesNS:  []float64{2600000, 2610000, 2620000, 2630000, 2645000, 2645138, 2660000, 2670000, 3184000, 3230000},
		ValidationPairedRatios: []float64{
			2700000.0 / 2600000, 2710000.0 / 2610000, 2720000.0 / 2620000, 2730000.0 / 2630000, 2742800.0 / 2645000,
			2742833.0 / 2645138, 2760000.0 / 2660000, 2770000.0 / 2670000, 2780000.0 / 3184000, 2790000.0 / 3230000,
		},
		ValidationControlMedianNS:   2_742_816.5,
		ValidationCandidateMedianNS: 2_645_069,
		ValidationRatioOfMedians:    2_742_816.5 / 2_645_069,
		CandidateTailSamplesNS:      []float64{3_184_000, 3_230_000},
		ControlAllocationBytes:      8,
		CandidateAllocationBytes:    8,
		ControlAllocationCount:      1,
		CandidateAllocationCount:    1,
		PromotionThreshold:          1.10,
		PromotionVerdict:            "fail",
		CandidateArchitectureScope:  "serial-encoder-persistence-only",
		ReferenceArchitectureBundle: []string{"dependency-aware concurrent dispatch", "buffer barriers", "graph fusion and optimization", "graph partitioning over multiple command buffers"},
		FullModelPromotionAllowed:   false,
		Classification:              "serial-component-only",
		FinalDecision:               "removed",
	}
	_ = evidence
	runMetalExecutorPersistentSerialEncoderFullGraphValidation()
}
