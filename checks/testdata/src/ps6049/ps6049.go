package ps6049

import "testing"

type streamingCounterProbeReport struct {
	XctraceToolVersion                    string
	OSIdentity                            string
	DeviceIdentity                        string
	MetalGPUCountersInstrumentAvailable   bool
	MetalGPUCountersInstrumentSelected    bool
	MetalApplicationInstrumentSelected    bool
	NullCounterSetAvoided                 bool
	FixedCommand                          string
	FixedArguments                        []string
	FixedIterationCount                   int
	BuffersPerIteration                   int
	CaptureDurationSeconds                float64
	RawExternalDirectory                  string
	CounterInfoTableCount                 int
	AllCounterInfoTablesEnumerated        bool
	RequiredCounterNames                  []string
	SemanticTableMatchCount               int
	UniqueSemanticTableSelected           bool
	ProfileNumberAndTableOrderIndependent bool
	RawCounterValueXMLBytes               int
	StreamingParserUsed                   bool
	PeakParserMemoryBytes                 int
	AllIDRefsResolved                     bool
	ObservedCommandBufferPairs            int
	ExpectedTimedCommandBufferPairs       int
	SelectedFinalCommandBufferPairs       int
	CommandBufferIDsPaired                bool
	AllSelectedCompletionsPresent         bool
	CalibrationAndWarmupExcluded          bool
	WallROIDurationNS                     float64
	UnionROIDurationNS                    float64
	CommandBufferDutyCycle                float64
	SelectedCounterNames                  []string
	CounterSampleCounts                   []float64
	CounterMinimums                       []float64
	CounterMeans                          []float64
	CounterMaximums                       []float64
	DeviceWideContaminationAcknowledged   bool
	CompactJSONHash                       string
	RawRetentionPolicy                    string
	RawCleanupCompleted                   bool
	Classification                        string
	FinalDecision                         string
}

func runXctraceMetalLimiterStreamingROIProbe() {}

func TestXctraceMetalLimiterStreamingROIProbeMissing(t *testing.T) { // want `xctrace Metal limiter probe has no streaming ROI manifest`
	runXctraceMetalLimiterStreamingROIProbe()
}

func TestXctraceMetalLimiterStreamingROIProbeIncomplete(t *testing.T) {
	evidence := streamingCounterProbeReport{ // want `xctrace Metal limiter evidence is incomplete; missing OS identity, device identity, Metal GPU Counters availability`
		XctraceToolVersion: "Xcode Instruments 16.0 xctrace",
	}
	_ = evidence
	runXctraceMetalLimiterStreamingROIProbe()
}

func TestXctraceMetalLimiterStreamingROIProbeInvalid(t *testing.T) {
	evidence := streamingCounterProbeReport{ // want `semantic counter-info discovery found 2 matching tables, want exactly one`
		XctraceToolVersion:                    "Xcode Instruments 16.0 xctrace",
		OSIdentity:                            "macOS 26.5.1",
		DeviceIdentity:                        "Apple M2 Pro",
		MetalGPUCountersInstrumentAvailable:   true,
		MetalGPUCountersInstrumentSelected:    true,
		MetalApplicationInstrumentSelected:    true,
		NullCounterSetAvoided:                 true,
		FixedCommand:                          "go test -bench BenchmarkQ4K",
		FixedArguments:                        []string{"-benchtime=10000x", "-count=1"},
		FixedIterationCount:                   10_000,
		BuffersPerIteration:                   1,
		CaptureDurationSeconds:                3.8,
		RawExternalDirectory:                  "/external/tmp/xctrace",
		CounterInfoTableCount:                 2,
		AllCounterInfoTablesEnumerated:        true,
		RequiredCounterNames:                  []string{"Buffer Read Limiter", "ALU Limiter", "Compute Occupancy"},
		SemanticTableMatchCount:               2,
		UniqueSemanticTableSelected:           true,
		ProfileNumberAndTableOrderIndependent: true,
		RawCounterValueXMLBytes:               358_000_000,
		StreamingParserUsed:                   true,
		PeakParserMemoryBytes:                 358_000_000,
		AllIDRefsResolved:                     true,
		ObservedCommandBufferPairs:            8_000,
		ExpectedTimedCommandBufferPairs:       9_999,
		SelectedFinalCommandBufferPairs:       9_000,
		CommandBufferIDsPaired:                true,
		AllSelectedCompletionsPresent:         true,
		CalibrationAndWarmupExcluded:          true,
		WallROIDurationNS:                     1_000_000_000,
		UnionROIDurationNS:                    1_100_000_000,
		CommandBufferDutyCycle:                0.5,
		SelectedCounterNames:                  []string{"Buffer Read Limiter", "ALU Limiter", "Compute Occupancy"},
		CounterSampleCounts:                   []float64{5_000, 0, 5_000},
		CounterMinimums:                       []float64{10, 20, 9},
		CounterMeans:                          []float64{19.65, 18.74, 7.30},
		CounterMaximums:                       []float64{30, 17, 12},
		DeviceWideContaminationAcknowledged:   true,
		CompactJSONHash:                       "sha256:compact",
		RawRetentionPolicy:                    "delete by default",
		RawCleanupCompleted:                   true,
		Classification:                        "invalid-roi",
		FinalDecision:                         "rejected",
	}
	_ = evidence
	runXctraceMetalLimiterStreamingROIProbe()
}

func TestXctraceMetalLimiterStreamingROIProbeStable(t *testing.T) {
	evidence := streamingCounterProbeReport{
		XctraceToolVersion:                    "Xcode Instruments 16.0 xctrace",
		OSIdentity:                            "macOS 26.5.1",
		DeviceIdentity:                        "Apple M2 Pro",
		MetalGPUCountersInstrumentAvailable:   true,
		MetalGPUCountersInstrumentSelected:    true,
		MetalApplicationInstrumentSelected:    true,
		NullCounterSetAvoided:                 true,
		FixedCommand:                          "go test -bench BenchmarkQ4K",
		FixedArguments:                        []string{"-benchtime=10000x", "-count=1"},
		FixedIterationCount:                   10_000,
		BuffersPerIteration:                   1,
		CaptureDurationSeconds:                3.8,
		RawExternalDirectory:                  "/external/tmp/xctrace",
		CounterInfoTableCount:                 2,
		AllCounterInfoTablesEnumerated:        true,
		RequiredCounterNames:                  []string{"Buffer Read Limiter", "ALU Limiter", "Compute Occupancy"},
		SemanticTableMatchCount:               1,
		UniqueSemanticTableSelected:           true,
		ProfileNumberAndTableOrderIndependent: true,
		RawCounterValueXMLBytes:               358_000_000,
		StreamingParserUsed:                   true,
		PeakParserMemoryBytes:                 8_000_000,
		AllIDRefsResolved:                     true,
		ObservedCommandBufferPairs:            10_041,
		ExpectedTimedCommandBufferPairs:       10_000,
		SelectedFinalCommandBufferPairs:       10_000,
		CommandBufferIDsPaired:                true,
		AllSelectedCompletionsPresent:         true,
		CalibrationAndWarmupExcluded:          true,
		WallROIDurationNS:                     1_000_000_000,
		UnionROIDurationNS:                    964_400_000,
		CommandBufferDutyCycle:                0.9644,
		SelectedCounterNames:                  []string{"Buffer Read Limiter", "ALU Limiter", "Compute Occupancy"},
		CounterSampleCounts:                   []float64{5_000, 5_000, 5_000},
		CounterMinimums:                       []float64{10, 9, 2},
		CounterMeans:                          []float64{19.65, 18.74, 5.30},
		CounterMaximums:                       []float64{30, 29, 12},
		DeviceWideContaminationAcknowledged:   true,
		CompactJSONHash:                       "sha256:compact",
		RawRetentionPolicy:                    "delete by default",
		RawCleanupCompleted:                   true,
		Classification:                        "validated-device-wide-roi",
		FinalDecision:                         "accepted-compact-evidence",
	}
	_ = evidence
	runXctraceMetalLimiterStreamingROIProbe()
}
