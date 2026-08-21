package ps6036

import "testing"

type xctraceCorrelationEvidence struct {
	TraceSchema                     string
	CounterSet                      string
	Hardware                        string
	BenchmarkDigest                 string
	RawReportHashes                 []string
	PhysicalCaptureCount            int
	SemanticTableDiscoveryPassed    bool
	RecursiveNestedReferencesPassed bool
	AmbiguousTablesRejected         bool
	TargetProcessID                 string
	TargetCommandBufferID           string
	ExactTargetIdentityPassed       bool
	TargetGPUSpanStartNS            float64
	TargetGPUSpanEndNS              float64
	ApplicationStageSpanNS          float64
	AlignmentScale                  float64
	SpanDisagreementNS              float64
	SpanToleranceNS                 float64
	AlignmentPassed                 bool
	StageLabels                     []string
	StageIntervalStartsNS           []float64
	StageIntervalEndsNS             []float64
	StageSampleCounts               []float64
	StageAggregateValues            []float64
	RequiredCounterNames            []string
	RequiredCounterSampleCounts     []float64
	MissingStageCount               int
	AllStagesReported               bool
	SubResolutionStageLabels        []string
	SubResolutionAggregationPolicy  string
	FabricatedInterpolation         bool
	ContaminationValue              float64
	ContaminationCeiling            float64
	ContaminationPassed             bool
	FreshProcessesIndependent       bool
	ReportAccepted                  bool
	RejectionReasons                []string
}

func runXctraceMetalGPUStageCounterCorrelation() {}

func TestXctraceMetalGPUStageCounterCorrelationMissing(t *testing.T) { // want `xctrace stage/counter harness has no target-scoped correlation manifest`
	runXctraceMetalGPUStageCounterCorrelation()
}

func TestXctraceMetalGPUStageCounterCorrelationIncomplete(t *testing.T) {
	evidence := xctraceCorrelationEvidence{ // want `xctrace correlation evidence is incomplete; missing counter set, hardware identity, benchmark digest`
		TraceSchema: "xctrace-26.6",
	}
	_ = evidence
	runXctraceMetalGPUStageCounterCorrelation()
}

func TestXctraceMetalGPUStageCounterCorrelationUnsafeAcceptance(t *testing.T) {
	evidence := xctraceCorrelationEvidence{ // want `span disagreement 2 ns exceeds declared 1 ns tolerance; report is accepted despite alignment drift; accepted report has zero samples for required counter "Threadgroup Limiter"`
		TraceSchema:                     "xctrace-26.6",
		CounterSet:                      "Performance Limiters + GPU",
		Hardware:                        "Apple M2 Pro",
		BenchmarkDigest:                 "tinyllama-exact",
		RawReportHashes:                 []string{"sha256-a", "sha256-b"},
		PhysicalCaptureCount:            2,
		SemanticTableDiscoveryPassed:    true,
		RecursiveNestedReferencesPassed: true,
		AmbiguousTablesRejected:         true,
		TargetProcessID:                 "pid-10",
		TargetCommandBufferID:           "cb-7",
		ExactTargetIdentityPassed:       true,
		TargetGPUSpanStartNS:            100,
		TargetGPUSpanEndNS:              500,
		ApplicationStageSpanNS:          400,
		AlignmentScale:                  1,
		SpanDisagreementNS:              2,
		SpanToleranceNS:                 1,
		AlignmentPassed:                 true,
		StageLabels:                     []string{"q4_k", "rmsnorm"},
		StageIntervalStartsNS:           []float64{100, 300},
		StageIntervalEndsNS:             []float64{300, 500},
		StageSampleCounts:               []float64{22, 4},
		StageAggregateValues:            []float64{0.8, 0.2},
		RequiredCounterNames:            []string{"ALU Limiter", "Threadgroup Limiter"},
		RequiredCounterSampleCounts:     []float64{10, 0},
		MissingStageCount:               1,
		AllStagesReported:               true,
		SubResolutionStageLabels:        []string{"rmsnorm"},
		SubResolutionAggregationPolicy:  "logical normalization family",
		FabricatedInterpolation:         false,
		ContaminationValue:              2,
		ContaminationCeiling:            1,
		ContaminationPassed:             true,
		FreshProcessesIndependent:       true,
		ReportAccepted:                  true,
		RejectionReasons:                []string{},
	}
	_ = evidence
	runXctraceMetalGPUStageCounterCorrelation()
}

func TestXctraceMetalGPUStageCounterCorrelationInterpolation(t *testing.T) {
	evidence := xctraceCorrelationEvidence{ // want `sub-resolution stage values use fabricated interpolation; sub-resolution stages lack an explicit logical aggregation policy`
		TraceSchema:                     "xctrace-26.6",
		CounterSet:                      "Performance Limiters + GPU",
		Hardware:                        "Apple M2 Pro",
		BenchmarkDigest:                 "tinyllama-exact",
		RawReportHashes:                 []string{"sha256-a"},
		PhysicalCaptureCount:            1,
		SemanticTableDiscoveryPassed:    true,
		RecursiveNestedReferencesPassed: true,
		AmbiguousTablesRejected:         true,
		TargetProcessID:                 "pid-10",
		TargetCommandBufferID:           "cb-7",
		ExactTargetIdentityPassed:       true,
		TargetGPUSpanStartNS:            100,
		TargetGPUSpanEndNS:              500,
		ApplicationStageSpanNS:          400,
		AlignmentScale:                  1,
		SpanDisagreementNS:              1,
		SpanToleranceNS:                 1,
		AlignmentPassed:                 true,
		StageLabels:                     []string{"q4_k", "rmsnorm"},
		StageIntervalStartsNS:           []float64{100, 300},
		StageIntervalEndsNS:             []float64{300, 500},
		StageSampleCounts:               []float64{22, 0},
		StageAggregateValues:            []float64{0.8, 0},
		RequiredCounterNames:            []string{"ALU Limiter"},
		RequiredCounterSampleCounts:     []float64{10},
		MissingStageCount:               0,
		AllStagesReported:               true,
		SubResolutionStageLabels:        []string{"rmsnorm"},
		SubResolutionAggregationPolicy:  "",
		FabricatedInterpolation:         true,
		ContaminationValue:              0,
		ContaminationCeiling:            1,
		ContaminationPassed:             true,
		FreshProcessesIndependent:       true,
		ReportAccepted:                  false,
		RejectionReasons:                []string{"fabricated interpolation"},
	}
	_ = evidence
	runXctraceMetalGPUStageCounterCorrelation()
}

func TestXctraceMetalGPUStageCounterCorrelationVectorMismatch(t *testing.T) {
	evidence := xctraceCorrelationEvidence{ // want `stage labels/intervals/sample-counts/aggregates have different lengths`
		TraceSchema:                     "xctrace-26.6",
		CounterSet:                      "Performance Limiters + GPU",
		Hardware:                        "Apple M2 Pro",
		BenchmarkDigest:                 "tinyllama-exact",
		RawReportHashes:                 []string{"sha256-a"},
		PhysicalCaptureCount:            1,
		SemanticTableDiscoveryPassed:    true,
		RecursiveNestedReferencesPassed: true,
		AmbiguousTablesRejected:         true,
		TargetProcessID:                 "pid-10",
		TargetCommandBufferID:           "cb-7",
		ExactTargetIdentityPassed:       true,
		TargetGPUSpanStartNS:            100,
		TargetGPUSpanEndNS:              500,
		ApplicationStageSpanNS:          400,
		AlignmentScale:                  1,
		SpanDisagreementNS:              1,
		SpanToleranceNS:                 1,
		AlignmentPassed:                 true,
		StageLabels:                     []string{"q4_k", "rmsnorm"},
		StageIntervalStartsNS:           []float64{100},
		StageIntervalEndsNS:             []float64{300, 500},
		StageSampleCounts:               []float64{22, 4},
		StageAggregateValues:            []float64{0.8, 0.2},
		RequiredCounterNames:            []string{"ALU Limiter"},
		RequiredCounterSampleCounts:     []float64{10},
		MissingStageCount:               0,
		AllStagesReported:               true,
		SubResolutionStageLabels:        []string{},
		SubResolutionAggregationPolicy:  "none",
		FabricatedInterpolation:         false,
		ContaminationValue:              0,
		ContaminationCeiling:            1,
		ContaminationPassed:             true,
		FreshProcessesIndependent:       true,
		ReportAccepted:                  true,
		RejectionReasons:                []string{},
	}
	_ = evidence
	runXctraceMetalGPUStageCounterCorrelation()
}

func TestXctraceMetalGPUStageCounterCorrelationStable(t *testing.T) {
	evidence := xctraceCorrelationEvidence{
		TraceSchema:                     "xctrace-26.6",
		CounterSet:                      "Performance Limiters + GPU",
		Hardware:                        "Apple M2 Pro",
		BenchmarkDigest:                 "tinyllama-exact",
		RawReportHashes:                 []string{"sha256-a", "sha256-b"},
		PhysicalCaptureCount:            2,
		SemanticTableDiscoveryPassed:    true,
		RecursiveNestedReferencesPassed: true,
		AmbiguousTablesRejected:         true,
		TargetProcessID:                 "pid-10",
		TargetCommandBufferID:           "cb-7",
		ExactTargetIdentityPassed:       true,
		TargetGPUSpanStartNS:            100,
		TargetGPUSpanEndNS:              500,
		ApplicationStageSpanNS:          400,
		AlignmentScale:                  1,
		SpanDisagreementNS:              1,
		SpanToleranceNS:                 1,
		AlignmentPassed:                 true,
		StageLabels:                     []string{"q4_k", "rmsnorm"},
		StageIntervalStartsNS:           []float64{100, 300},
		StageIntervalEndsNS:             []float64{300, 500},
		StageSampleCounts:               []float64{22, 4},
		StageAggregateValues:            []float64{0.8, 0.2},
		RequiredCounterNames:            []string{"ALU Limiter", "Threadgroup Limiter"},
		RequiredCounterSampleCounts:     []float64{10, 8},
		MissingStageCount:               0,
		AllStagesReported:               true,
		SubResolutionStageLabels:        []string{"rmsnorm"},
		SubResolutionAggregationPolicy:  "logical normalization family",
		FabricatedInterpolation:         false,
		ContaminationValue:              0,
		ContaminationCeiling:            1,
		ContaminationPassed:             true,
		FreshProcessesIndependent:       true,
		ReportAccepted:                  true,
		RejectionReasons:                []string{},
	}
	_ = evidence
	runXctraceMetalGPUStageCounterCorrelation()
}
