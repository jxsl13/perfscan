package ps6037

import "testing"

type xctraceCapturePolicy struct {
	ProfilerIdentity           string
	EnvironmentAllowlist       []string
	ForwardedEnvironmentNames  []string
	SourceInputPath            string
	StagedInputPath            string
	SourceSHA256               string
	StagedSHA256               string
	StagingPassed              bool
	TraceSafePathPassed        bool
	CaptureCommandBufferLimit  int
	CaptureCommandBufferCount  int
	CaptureByteEstimate        float64
	CaptureByteLimit           float64
	ExportByteEstimate         float64
	ExportByteLimit            float64
	RawTracePath               string
	RawTraceOutsideRepository  bool
	CompactReportPath          string
	CompactReportOnlyPublished bool
	CounterNames               []string
	CounterPolicies            []string
	CounterSampleCounts        []float64
	CounterActiveMeans         []float64
	CounterCeilings            []float64
	RequiredAcceptedReports    int
	AttemptedCaptures          int
	RejectedCaptures           int
	AcceptedCaptures           int
	CaptureRejectionReasons    []string
	ReportsIndependent         bool
	MedianAggregationPassed    bool
	ResultEmitted              bool
}

func runXctraceMetalGPUCapturePolicy() {}

func TestXctraceMetalGPUCapturePolicyMissing(t *testing.T) { // want `external GPU-profiler harness has no bounded capture-policy manifest`
	runXctraceMetalGPUCapturePolicy()
}

func TestXctraceMetalGPUCapturePolicyIncomplete(t *testing.T) {
	evidence := xctraceCapturePolicy{ // want `external profiler capture evidence is incomplete; missing environment allowlist, forwarded environment names, source input path`
		ProfilerIdentity: "xctrace 26.6",
	}
	_ = evidence
	runXctraceMetalGPUCapturePolicy()
}

func TestXctraceMetalGPUCapturePolicyUnsafeEmission(t *testing.T) {
	evidence := xctraceCapturePolicy{ // want `forwarded environment variable "HOME" is outside the explicit allowlist; source and staged SHA-256 digests differ; capture command-buffer estimate/count 200 exceeds 1 limit`
		ProfilerIdentity:           "xctrace 26.6",
		EnvironmentAllowlist:       []string{"MODEL_PATH"},
		ForwardedEnvironmentNames:  []string{"MODEL_PATH", "HOME"},
		SourceInputPath:            "/Users/test/Desktop/model.gguf",
		StagedInputPath:            "/tmp/trace/model.gguf",
		SourceSHA256:               "sha256-source",
		StagedSHA256:               "sha256-different",
		StagingPassed:              true,
		TraceSafePathPassed:        true,
		CaptureCommandBufferLimit:  1,
		CaptureCommandBufferCount:  200,
		CaptureByteEstimate:        3e9,
		CaptureByteLimit:           2e8,
		ExportByteEstimate:         5e9,
		ExportByteLimit:            2e8,
		RawTracePath:               "/tmp/traces/run.trace",
		RawTraceOutsideRepository:  true,
		CompactReportPath:          "reports/run.json",
		CompactReportOnlyPublished: true,
		CounterNames:               []string{"Fragment Occupancy", "Buffer Read Limiter"},
		CounterPolicies:            []string{"contamination-sentinel", "required"},
		CounterSampleCounts:        []float64{10, 0},
		CounterActiveMeans:         []float64{0.02188452, 0},
		CounterCeilings:            []float64{0.001, 1},
		RequiredAcceptedReports:    5,
		AttemptedCaptures:          3,
		RejectedCaptures:           1,
		AcceptedCaptures:           1,
		CaptureRejectionReasons:    []string{"contamination"},
		ReportsIndependent:         true,
		MedianAggregationPassed:    true,
		ResultEmitted:              true,
	}
	_ = evidence
	runXctraceMetalGPUCapturePolicy()
}

func TestXctraceMetalGPUCapturePolicyStorageFailure(t *testing.T) {
	evidence := xctraceCapturePolicy{ // want `input staging is explicitly false; trace-safe path is explicitly false; raw-trace external retention is explicitly false; compact-only publication is explicitly false`
		ProfilerIdentity:           "xctrace 26.6",
		EnvironmentAllowlist:       []string{"MODEL_PATH"},
		ForwardedEnvironmentNames:  []string{"MODEL_PATH"},
		SourceInputPath:            "/Users/test/Desktop/model.gguf",
		StagedInputPath:            "/tmp/trace/model.gguf",
		SourceSHA256:               "sha256-same",
		StagedSHA256:               "sha256-same",
		StagingPassed:              false,
		TraceSafePathPassed:        false,
		CaptureCommandBufferLimit:  1,
		CaptureCommandBufferCount:  1,
		CaptureByteEstimate:        1e8,
		CaptureByteLimit:           2e8,
		ExportByteEstimate:         1e8,
		ExportByteLimit:            2e8,
		RawTracePath:               "traces/run.trace",
		RawTraceOutsideRepository:  false,
		CompactReportPath:          "reports/run.json",
		CompactReportOnlyPublished: false,
		CounterNames:               []string{"Buffer Read Limiter"},
		CounterPolicies:            []string{"required"},
		CounterSampleCounts:        []float64{100},
		CounterActiveMeans:         []float64{0.77},
		CounterCeilings:            []float64{1},
		RequiredAcceptedReports:    1,
		AttemptedCaptures:          1,
		RejectedCaptures:           0,
		AcceptedCaptures:           1,
		CaptureRejectionReasons:    []string{},
		ReportsIndependent:         true,
		MedianAggregationPassed:    true,
		ResultEmitted:              true,
	}
	_ = evidence
	runXctraceMetalGPUCapturePolicy()
}

func TestXctraceMetalGPUCapturePolicyVectorMismatch(t *testing.T) {
	evidence := xctraceCapturePolicy{ // want `counter names/policies/sample-counts/active-means/ceilings have different lengths`
		ProfilerIdentity:           "xctrace 26.6",
		EnvironmentAllowlist:       []string{"MODEL_PATH"},
		ForwardedEnvironmentNames:  []string{"MODEL_PATH"},
		SourceInputPath:            "/Users/test/Desktop/model.gguf",
		StagedInputPath:            "/tmp/trace/model.gguf",
		SourceSHA256:               "sha256-same",
		StagedSHA256:               "sha256-same",
		StagingPassed:              true,
		TraceSafePathPassed:        true,
		CaptureCommandBufferLimit:  1,
		CaptureCommandBufferCount:  1,
		CaptureByteEstimate:        1e8,
		CaptureByteLimit:           2e8,
		ExportByteEstimate:         1e8,
		ExportByteLimit:            2e8,
		RawTracePath:               "/tmp/traces/run.trace",
		RawTraceOutsideRepository:  true,
		CompactReportPath:          "reports/run.json",
		CompactReportOnlyPublished: true,
		CounterNames:               []string{"Fragment Occupancy", "Buffer Read Limiter"},
		CounterPolicies:            []string{"contamination-sentinel"},
		CounterSampleCounts:        []float64{100, 100},
		CounterActiveMeans:         []float64{0, 0.77},
		CounterCeilings:            []float64{0.001, 1},
		RequiredAcceptedReports:    1,
		AttemptedCaptures:          1,
		RejectedCaptures:           0,
		AcceptedCaptures:           1,
		CaptureRejectionReasons:    []string{},
		ReportsIndependent:         true,
		MedianAggregationPassed:    true,
		ResultEmitted:              true,
	}
	_ = evidence
	runXctraceMetalGPUCapturePolicy()
}

func TestXctraceMetalGPUCapturePolicyStable(t *testing.T) {
	evidence := xctraceCapturePolicy{
		ProfilerIdentity:           "xctrace 26.6",
		EnvironmentAllowlist:       []string{"MODEL_PATH"},
		ForwardedEnvironmentNames:  []string{"MODEL_PATH"},
		SourceInputPath:            "/Users/test/Desktop/model.gguf",
		StagedInputPath:            "/tmp/trace/model.gguf",
		SourceSHA256:               "sha256-same",
		StagedSHA256:               "sha256-same",
		StagingPassed:              true,
		TraceSafePathPassed:        true,
		CaptureCommandBufferLimit:  1,
		CaptureCommandBufferCount:  1,
		CaptureByteEstimate:        1e8,
		CaptureByteLimit:           2e8,
		ExportByteEstimate:         1e8,
		ExportByteLimit:            2e8,
		RawTracePath:               "/tmp/traces/run.trace",
		RawTraceOutsideRepository:  true,
		CompactReportPath:          "reports/run.json",
		CompactReportOnlyPublished: true,
		CounterNames:               []string{"Fragment Occupancy", "Texture Sample Limiter", "Buffer Read Limiter"},
		CounterPolicies:            []string{"contamination-sentinel", "contamination-sentinel", "required"},
		CounterSampleCounts:        []float64{2000, 2000, 2000},
		CounterActiveMeans:         []float64{0.0005, 0.0004, 0.77028616},
		CounterCeilings:            []float64{0.001, 0.001, 1},
		RequiredAcceptedReports:    5,
		AttemptedCaptures:          7,
		RejectedCaptures:           2,
		AcceptedCaptures:           5,
		CaptureRejectionReasons:    []string{"Fragment Occupancy 2.188452%", "Fragment Occupancy 0.121756%"},
		ReportsIndependent:         true,
		MedianAggregationPassed:    true,
		ResultEmitted:              true,
	}
	_ = evidence
	runXctraceMetalGPUCapturePolicy()
}
