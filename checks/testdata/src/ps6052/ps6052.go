package ps6052

import "testing"

type parameterArenaEvidence struct {
	Hardware                          string
	ToolchainIdentity                 string
	CandidateDefaultOff               bool
	ImmutableParameterBufferCount     int
	MaximumImmutableRecordBytes       int
	ArenaCapacityBytes                int
	ArenaLivePayloadBytes             int
	ArenaUtilization                  float64
	AlignmentBytes                    int
	SuballocationAlignmentVerified    bool
	OverflowFallbackVerified          bool
	AsyncCommitWaitLifetimeVerified   bool
	ProfilingRecorderCoverageVerified bool
	ArenaAllocatedPerRecorder         bool
	CompletionAwarePoolDocumented     bool
	LongLivedInFlightRingDocumented   bool
	InFlightEpochCount                int
	SetBytesDriverValidated           bool
	HostProcessCount                  int
	HostIterationCountPerProcess      int
	HostAlternatingOrder              bool
	HostControlLatenciesNS            []float64
	HostCandidateLatenciesNS          []float64
	HostControlMedianNS               float64
	HostCandidateMedianNS             float64
	HostControlCandidateRatio         float64
	HostControlAllocationCount        int
	HostCandidateAllocationCount      int
	EndToEndWorkload                  string
	EndToEndTokenCount                int
	EndToEndProcessCount              int
	EndToEndIterationCountPerProcess  int
	EndToEndAlternatingOrder          bool
	EndToEndControlLatenciesNS        []float64
	EndToEndCandidateLatenciesNS      []float64
	EndToEndControlMedianNS           float64
	EndToEndCandidateMedianNS         float64
	EndToEndControlCandidateRatio     float64
	PairedCandidateControlRatios      []float64
	MedianPairedCandidateControlRatio float64
	ExactOutputParity                 bool
	OutputMismatchCount               int
	EndToEndPerformanceMinimum        float64
	EndToEndPerformanceVerdict        string
	PerRecorderCandidateRecommended   bool
	AmortizedDesignRecommended        bool
	Classification                    string
	FinalDecision                     string
}

func runMetalImmutableParameterBufferArenaRecording() {}

func TestMetalImmutableParameterBufferArenaRecordingMissing(t *testing.T) { // want `Metal immutable-parameter arena has no amortization manifest`
	runMetalImmutableParameterBufferArenaRecording()
}

func TestMetalImmutableParameterBufferArenaRecordingIncomplete(t *testing.T) {
	evidence := parameterArenaEvidence{ // want `Metal parameter-arena evidence is incomplete; missing toolchain identity, candidate default-off status, immutable buffer count`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalImmutableParameterBufferArenaRecording()
}

func TestMetalImmutableParameterBufferArenaRecordingOverrecommend(t *testing.T) {
	evidence := parameterArenaEvidence{ // want `complete-workload performance verdict "pass" disagrees with computed gate; per-recorder arena is recommended despite failed complete-workload or non-amortized lifetime gates; final decision "retained" retains a per-recorder arena that failed the complete-workload/amortization gate`
		Hardware:                          "Apple M2 Pro",
		ToolchainIdentity:                 "Xcode 26.6 Metal",
		CandidateDefaultOff:               true,
		ImmutableParameterBufferCount:     32,
		MaximumImmutableRecordBytes:       64,
		ArenaCapacityBytes:                256 * 1024,
		ArenaLivePayloadBytes:             2048,
		ArenaUtilization:                  2048.0 / (256 * 1024),
		AlignmentBytes:                    256,
		SuballocationAlignmentVerified:    true,
		OverflowFallbackVerified:          true,
		AsyncCommitWaitLifetimeVerified:   true,
		ProfilingRecorderCoverageVerified: true,
		ArenaAllocatedPerRecorder:         true,
		CompletionAwarePoolDocumented:     true,
		LongLivedInFlightRingDocumented:   true,
		InFlightEpochCount:                3,
		SetBytesDriverValidated:           false,
		HostProcessCount:                  10,
		HostIterationCountPerProcess:      500,
		HostAlternatingOrder:              true,
		HostControlLatenciesNS:            []float64{84000, 84500, 85000, 85500, 85799, 85800, 86000, 86500, 87000, 87500},
		HostCandidateLatenciesNS:          []float64{27000, 27500, 27800, 28000, 28103, 28104, 28300, 28500, 29000, 29500},
		HostControlMedianNS:               85799.5,
		HostCandidateMedianNS:             28103.5,
		HostControlCandidateRatio:         85799.5 / 28103.5,
		HostControlAllocationCount:        1,
		HostCandidateAllocationCount:      1,
		EndToEndWorkload:                  "TinyLlama Q4_K_M complete 2-token generation",
		EndToEndTokenCount:                2,
		EndToEndProcessCount:              10,
		EndToEndIterationCountPerProcess:  500,
		EndToEndAlternatingOrder:          true,
		EndToEndControlLatenciesNS:        []float64{70000000, 72000000, 74000000, 75000000, 75200000, 75236000, 76000000, 78000000, 90000000, 100000000},
		EndToEndCandidateLatenciesNS:      []float64{80000000, 90000000, 77941733, 76000000, 100000000, 72000000, 82000000, 74000000, 84000000, 80156000},
		EndToEndControlMedianNS:           75218000,
		EndToEndCandidateMedianNS:         80078000,
		EndToEndControlCandidateRatio:     75218000.0 / 80078000,
		PairedCandidateControlRatios:      []float64{80000000.0 / 70000000, 90000000.0 / 72000000, 77941733.0 / 74000000, 76000000.0 / 75000000, 100000000.0 / 75200000, 72000000.0 / 75236000, 82000000.0 / 76000000, 74000000.0 / 78000000, 84000000.0 / 90000000, 80156000.0 / 100000000},
		MedianPairedCandidateControlRatio: 1.033299997747748,
		ExactOutputParity:                 true,
		OutputMismatchCount:               0,
		EndToEndPerformanceMinimum:        1,
		EndToEndPerformanceVerdict:        "pass",
		PerRecorderCandidateRecommended:   true,
		AmortizedDesignRecommended:        true,
		Classification:                    "host-leaf-win-end-to-end-regression",
		FinalDecision:                     "retained",
	}
	_ = evidence
	runMetalImmutableParameterBufferArenaRecording()
}

func TestMetalImmutableParameterBufferArenaRecordingStale(t *testing.T) {
	evidence := parameterArenaEvidence{ // want `host control/candidate ratio 9 disagrees with 3.05298`
		Hardware:                          "Apple M2 Pro",
		ToolchainIdentity:                 "Xcode 26.6 Metal",
		CandidateDefaultOff:               true,
		ImmutableParameterBufferCount:     32,
		MaximumImmutableRecordBytes:       64,
		ArenaCapacityBytes:                256 * 1024,
		ArenaLivePayloadBytes:             2048,
		ArenaUtilization:                  2048.0 / (256 * 1024),
		AlignmentBytes:                    256,
		SuballocationAlignmentVerified:    true,
		OverflowFallbackVerified:          true,
		AsyncCommitWaitLifetimeVerified:   true,
		ProfilingRecorderCoverageVerified: true,
		ArenaAllocatedPerRecorder:         true,
		CompletionAwarePoolDocumented:     true,
		LongLivedInFlightRingDocumented:   true,
		InFlightEpochCount:                3,
		SetBytesDriverValidated:           false,
		HostProcessCount:                  10,
		HostIterationCountPerProcess:      500,
		HostAlternatingOrder:              true,
		HostControlLatenciesNS:            []float64{84000, 84500, 85000, 85500, 85799, 85800, 86000, 86500, 87000, 87500},
		HostCandidateLatenciesNS:          []float64{27000, 27500, 27800, 28000, 28103, 28104, 28300, 28500, 29000, 29500},
		HostControlMedianNS:               85799.5,
		HostCandidateMedianNS:             28103.5,
		HostControlCandidateRatio:         9,
		HostControlAllocationCount:        1,
		HostCandidateAllocationCount:      1,
		EndToEndWorkload:                  "TinyLlama Q4_K_M complete 2-token generation",
		EndToEndTokenCount:                2,
		EndToEndProcessCount:              10,
		EndToEndIterationCountPerProcess:  500,
		EndToEndAlternatingOrder:          true,
		EndToEndControlLatenciesNS:        []float64{70000000, 72000000, 74000000, 75000000, 75200000, 75236000, 76000000, 78000000, 90000000, 100000000},
		EndToEndCandidateLatenciesNS:      []float64{80000000, 90000000, 77941733, 76000000, 100000000, 72000000, 82000000, 74000000, 84000000, 80156000},
		EndToEndControlMedianNS:           75218000,
		EndToEndCandidateMedianNS:         80078000,
		EndToEndControlCandidateRatio:     75218000.0 / 80078000,
		PairedCandidateControlRatios:      []float64{80000000.0 / 70000000, 90000000.0 / 72000000, 77941733.0 / 74000000, 76000000.0 / 75000000, 100000000.0 / 75200000, 72000000.0 / 75236000, 82000000.0 / 76000000, 74000000.0 / 78000000, 84000000.0 / 90000000, 80156000.0 / 100000000},
		MedianPairedCandidateControlRatio: 1.033299997747748,
		ExactOutputParity:                 true,
		OutputMismatchCount:               0,
		EndToEndPerformanceMinimum:        1,
		EndToEndPerformanceVerdict:        "fail",
		PerRecorderCandidateRecommended:   false,
		AmortizedDesignRecommended:        true,
		Classification:                    "host-leaf-win-end-to-end-regression",
		FinalDecision:                     "reject-per-recorder-arena-pursue-completion-aware-pool",
	}
	_ = evidence
	runMetalImmutableParameterBufferArenaRecording()
}

func TestMetalImmutableParameterBufferArenaRecordingStable(t *testing.T) {
	evidence := parameterArenaEvidence{
		Hardware:                          "Apple M2 Pro",
		ToolchainIdentity:                 "Xcode 26.6 Metal",
		CandidateDefaultOff:               true,
		ImmutableParameterBufferCount:     32,
		MaximumImmutableRecordBytes:       64,
		ArenaCapacityBytes:                256 * 1024,
		ArenaLivePayloadBytes:             2048,
		ArenaUtilization:                  2048.0 / (256 * 1024),
		AlignmentBytes:                    256,
		SuballocationAlignmentVerified:    true,
		OverflowFallbackVerified:          true,
		AsyncCommitWaitLifetimeVerified:   true,
		ProfilingRecorderCoverageVerified: true,
		ArenaAllocatedPerRecorder:         true,
		CompletionAwarePoolDocumented:     true,
		LongLivedInFlightRingDocumented:   true,
		InFlightEpochCount:                3,
		SetBytesDriverValidated:           false,
		HostProcessCount:                  10,
		HostIterationCountPerProcess:      500,
		HostAlternatingOrder:              true,
		HostControlLatenciesNS:            []float64{84000, 84500, 85000, 85500, 85799, 85800, 86000, 86500, 87000, 87500},
		HostCandidateLatenciesNS:          []float64{27000, 27500, 27800, 28000, 28103, 28104, 28300, 28500, 29000, 29500},
		HostControlMedianNS:               85799.5,
		HostCandidateMedianNS:             28103.5,
		HostControlCandidateRatio:         85799.5 / 28103.5,
		HostControlAllocationCount:        1,
		HostCandidateAllocationCount:      1,
		EndToEndWorkload:                  "TinyLlama Q4_K_M complete 2-token generation",
		EndToEndTokenCount:                2,
		EndToEndProcessCount:              10,
		EndToEndIterationCountPerProcess:  500,
		EndToEndAlternatingOrder:          true,
		EndToEndControlLatenciesNS:        []float64{70000000, 72000000, 74000000, 75000000, 75200000, 75236000, 76000000, 78000000, 90000000, 100000000},
		EndToEndCandidateLatenciesNS:      []float64{80000000, 90000000, 77941733, 76000000, 100000000, 72000000, 82000000, 74000000, 84000000, 80156000},
		EndToEndControlMedianNS:           75218000,
		EndToEndCandidateMedianNS:         80078000,
		EndToEndControlCandidateRatio:     75218000.0 / 80078000,
		PairedCandidateControlRatios:      []float64{80000000.0 / 70000000, 90000000.0 / 72000000, 77941733.0 / 74000000, 76000000.0 / 75000000, 100000000.0 / 75200000, 72000000.0 / 75236000, 82000000.0 / 76000000, 74000000.0 / 78000000, 84000000.0 / 90000000, 80156000.0 / 100000000},
		MedianPairedCandidateControlRatio: 1.033299997747748,
		ExactOutputParity:                 true,
		OutputMismatchCount:               0,
		EndToEndPerformanceMinimum:        1,
		EndToEndPerformanceVerdict:        "fail",
		PerRecorderCandidateRecommended:   false,
		AmortizedDesignRecommended:        true,
		Classification:                    "host-leaf-win-end-to-end-regression",
		FinalDecision:                     "reject-per-recorder-arena-pursue-completion-aware-pool",
	}
	_ = evidence
	runMetalImmutableParameterBufferArenaRecording()
}
