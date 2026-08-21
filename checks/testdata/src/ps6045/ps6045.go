package ps6045

import "testing"

type profilerInitializationEvidence struct {
	Hardware                                      string
	OSIdentity                                    string
	SourceRevision                                string
	CompilerIdentity                              string
	MetalLibraryMode                              string
	PipelineCacheState                            string
	CaptureToolVersion                            string
	StandaloneInitializationMS                    float64
	ProfiledInitializationMS                      float64
	ProfiledInitializationCensored                bool
	InitializationInflationRatio                  float64
	MaxInitializationInflationRatio               float64
	CaptureDurationSeconds                        float64
	TimedRegionMarker                             string
	TimedRegionReached                            bool
	TargetProgressOutput                          string
	WarmupCompleted                               bool
	CaptureActivatedAfterWarmup                   bool
	InProcessCaptureTriggerUsed                   bool
	CaptureBoundary                               string
	EncoderSubmissionSchemasPresent               bool
	PerKernelTimingAccepted                       bool
	BenchmarkSamplesIncludeProfiledInitialization bool
	CaptureClassification                         string
	FinalDecision                                 string
}

func runMetalGPUProfilerColdLazyPipelineInitialization() {}

func TestMetalGPUProfilerColdLazyPipelineInitializationMissing(t *testing.T) { // want `GPU lazy-pipeline capture has no timed-region trace gate`
	runMetalGPUProfilerColdLazyPipelineInitialization()
}

func TestMetalGPUProfilerColdLazyPipelineInitializationIncomplete(t *testing.T) {
	evidence := profilerInitializationEvidence{ // want `GPU profiler initialization evidence is incomplete; missing OS identity, source revision, compiler identity`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUProfilerColdLazyPipelineInitialization()
}

func TestMetalGPUProfilerColdLazyPipelineInitializationUnsafeTiming(t *testing.T) {
	evidence := profilerInitializationEvidence{ // want `profiled initialization is mixed into benchmark samples; per-kernel timing is accepted although the timed-region marker was not reached; per-kernel timing is accepted from censored initialization; per-kernel timing is accepted despite 11428.6x initialization inflation exceeding 2x; per-kernel timing is accepted without a prewarmed in-process activation boundary; capture classification "timing-evidence" does not label a cold-launch trace topology-only; final decision "accept-timing" accepts timing from a distorted cold capture`
		Hardware:                                      "Apple M2 Pro",
		OSIdentity:                                    "macOS 26.5.1",
		SourceRevision:                                "llama.cpp 4df29be4",
		CompilerIdentity:                              "Xcode Metal compiler",
		MetalLibraryMode:                              "embedded library with lazy pipelines",
		PipelineCacheState:                            "cold empty cache",
		CaptureToolVersion:                            "Xcode Instruments 26.5.1 Metal Application",
		StandaloneInitializationMS:                    10.5,
		ProfiledInitializationMS:                      120_000,
		ProfiledInitializationCensored:                true,
		InitializationInflationRatio:                  120_000.0 / 10.5,
		MaxInitializationInflationRatio:               2,
		CaptureDurationSeconds:                        120,
		TimedRegionMarker:                             "llama-bench generation-loop-start",
		TimedRegionReached:                            false,
		TargetProgressOutput:                          "stopped during pipeline initialization",
		WarmupCompleted:                               false,
		CaptureActivatedAfterWarmup:                   false,
		InProcessCaptureTriggerUsed:                   false,
		CaptureBoundary:                               "external cold process launch",
		EncoderSubmissionSchemasPresent:               true,
		PerKernelTimingAccepted:                       true,
		BenchmarkSamplesIncludeProfiledInitialization: true,
		CaptureClassification:                         "timing-evidence",
		FinalDecision:                                 "accept-timing",
	}
	_ = evidence
	runMetalGPUProfilerColdLazyPipelineInitialization()
}

func TestMetalGPUProfilerColdLazyPipelineInitializationStaleRatio(t *testing.T) {
	evidence := profilerInitializationEvidence{ // want `initialization inflation ratio 2x disagrees with profiled/standalone 11428.6x`
		Hardware:                                      "Apple M2 Pro",
		OSIdentity:                                    "macOS 26.5.1",
		SourceRevision:                                "llama.cpp 4df29be4",
		CompilerIdentity:                              "Xcode Metal compiler",
		MetalLibraryMode:                              "embedded library with lazy pipelines",
		PipelineCacheState:                            "cold empty cache",
		CaptureToolVersion:                            "Xcode Instruments 26.5.1 Metal Application",
		StandaloneInitializationMS:                    10.5,
		ProfiledInitializationMS:                      120_000,
		ProfiledInitializationCensored:                true,
		InitializationInflationRatio:                  2,
		MaxInitializationInflationRatio:               2,
		CaptureDurationSeconds:                        120,
		TimedRegionMarker:                             "llama-bench generation-loop-start",
		TimedRegionReached:                            false,
		TargetProgressOutput:                          "stopped during pipeline initialization",
		WarmupCompleted:                               false,
		CaptureActivatedAfterWarmup:                   false,
		InProcessCaptureTriggerUsed:                   false,
		CaptureBoundary:                               "external cold process launch",
		EncoderSubmissionSchemasPresent:               true,
		PerKernelTimingAccepted:                       false,
		BenchmarkSamplesIncludeProfiledInitialization: false,
		CaptureClassification:                         "topology-only",
		FinalDecision:                                 "timing-rejected",
	}
	_ = evidence
	runMetalGPUProfilerColdLazyPipelineInitialization()
}

func TestMetalGPUProfilerColdLazyPipelineInitializationStable(t *testing.T) {
	evidence := profilerInitializationEvidence{
		Hardware:                                      "Apple M2 Pro",
		OSIdentity:                                    "macOS 26.5.1",
		SourceRevision:                                "llama.cpp 4df29be4",
		CompilerIdentity:                              "Xcode Metal compiler",
		MetalLibraryMode:                              "embedded library with lazy pipelines",
		PipelineCacheState:                            "cold empty cache",
		CaptureToolVersion:                            "Xcode Instruments 26.5.1 Metal Application",
		StandaloneInitializationMS:                    10.5,
		ProfiledInitializationMS:                      120_000,
		ProfiledInitializationCensored:                true,
		InitializationInflationRatio:                  120_000.0 / 10.5,
		MaxInitializationInflationRatio:               2,
		CaptureDurationSeconds:                        120,
		TimedRegionMarker:                             "llama-bench generation-loop-start",
		TimedRegionReached:                            false,
		TargetProgressOutput:                          "stopped during pipeline initialization",
		WarmupCompleted:                               false,
		CaptureActivatedAfterWarmup:                   false,
		InProcessCaptureTriggerUsed:                   false,
		CaptureBoundary:                               "external cold process launch",
		EncoderSubmissionSchemasPresent:               true,
		PerKernelTimingAccepted:                       false,
		BenchmarkSamplesIncludeProfiledInitialization: false,
		CaptureClassification:                         "topology-only",
		FinalDecision:                                 "timing-rejected",
	}
	_ = evidence
	runMetalGPUProfilerColdLazyPipelineInitialization()
}
