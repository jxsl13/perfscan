package ps6030

import "testing"

type sampleDensityEvidence struct {
	Hardware                          string
	WorkloadDigest                    string
	GraphIdentity                     string
	GlobalEffectiveSampleCounts       []float64
	GlobalActiveMicroseconds          []float64
	GlobalSamplesPerActiveMicrosecond []float64
	GlobalMinimumSampleCount          float64
	GlobalMinimumDensity              float64
	StageNames                        []string
	StageEffectiveSampleCounts        []float64
	StageActiveMicroseconds           []float64
	StageSamplesPerActiveMicrosecond  []float64
	StageMinimumSampleCounts          []float64
	StageMinimumDensities             []float64
	StageDensityStatuses              []string
	AcceptedProcessIDs                []string
	AcceptedProcessSampleDensities    []float64
	RequiredAcceptedProcessCount      int
	AcceptedProcessCount              int
	IndependentProcesses              bool
	IdentityGatePassed                bool
	ContaminationGatePassed           bool
	CompletenessGatePassed            bool
	DensityGatePassed                 bool
	AggregateOnlyFullyGatedProcesses  bool
	LowDensityRetainedDiagnostic      bool
	SamplingCadenceVarianceRatio      float64
	SamplingCadenceVarianceDisclosed  bool
	ExactOutputDigestPassed           bool
	ExactEventTopologyPassed          bool
	MediansPublished                  bool
	CandidateSelected                 bool
}

func runMetalGPUProfilerStageSampleDensityGate() {}

func TestMetalGPUProfilerStageSampleDensityMissing(t *testing.T) { // want `profiler sample-density harness has no density-gate manifest`
	runMetalGPUProfilerStageSampleDensityGate()
}

func TestMetalGPUProfilerStageSampleDensityIncomplete(t *testing.T) {
	evidence := sampleDensityEvidence{ // want `profiler sample-density evidence is incomplete; missing workload digest, graph identity, global effective sample counts`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUProfilerStageSampleDensityGate()
}

func TestMetalGPUProfilerStageSampleDensitySparseAccepted(t *testing.T) {
	evidence := sampleDensityEvidence{ // want `stage "q6_k" is below its count/density floor but status is "sufficient"; density gate passes despite a required global/stage scope below its floor; low-density stream is not retained as an insufficient-density diagnostic`
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		GlobalEffectiveSampleCounts:       []float64{1384, 230},
		GlobalActiveMicroseconds:          []float64{1000, 1000},
		GlobalSamplesPerActiveMicrosecond: []float64{1.384, 0.23},
		GlobalMinimumSampleCount:          200,
		GlobalMinimumDensity:              0.2,
		StageNames:                        []string{"residual+rmsnorm", "q6_k"},
		StageEffectiveSampleCounts:        []float64{22, 4},
		StageActiveMicroseconds:           []float64{100, 100},
		StageSamplesPerActiveMicrosecond:  []float64{0.22, 0.04},
		StageMinimumSampleCounts:          []float64{10, 10},
		StageMinimumDensities:             []float64{0.1, 0.1},
		StageDensityStatuses:              []string{"sufficient", "sufficient"},
		AcceptedProcessIDs:                []string{"process-1", "process-2"},
		AcceptedProcessSampleDensities:    []float64{1.384, 0.23},
		RequiredAcceptedProcessCount:      2,
		AcceptedProcessCount:              2,
		IndependentProcesses:              true,
		IdentityGatePassed:                true,
		ContaminationGatePassed:           true,
		CompletenessGatePassed:            true,
		DensityGatePassed:                 true,
		AggregateOnlyFullyGatedProcesses:  true,
		LowDensityRetainedDiagnostic:      false,
		SamplingCadenceVarianceRatio:      6.017391304,
		SamplingCadenceVarianceDisclosed:  true,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  true,
		CandidateSelected:                 true,
	}
	_ = evidence
	runMetalGPUProfilerStageSampleDensityGate()
}

func TestMetalGPUProfilerStageSampleDensitySparseDiagnostic(t *testing.T) {
	evidence := sampleDensityEvidence{
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		GlobalEffectiveSampleCounts:       []float64{1384, 230},
		GlobalActiveMicroseconds:          []float64{1000, 1000},
		GlobalSamplesPerActiveMicrosecond: []float64{1.384, 0.23},
		GlobalMinimumSampleCount:          200,
		GlobalMinimumDensity:              0.2,
		StageNames:                        []string{"residual+rmsnorm", "q6_k"},
		StageEffectiveSampleCounts:        []float64{22, 4},
		StageActiveMicroseconds:           []float64{100, 100},
		StageSamplesPerActiveMicrosecond:  []float64{0.22, 0.04},
		StageMinimumSampleCounts:          []float64{10, 10},
		StageMinimumDensities:             []float64{0.1, 0.1},
		StageDensityStatuses:              []string{"sufficient", "insufficient-density"},
		AcceptedProcessIDs:                []string{"process-1", "process-2"},
		AcceptedProcessSampleDensities:    []float64{1.384, 0.23},
		RequiredAcceptedProcessCount:      2,
		AcceptedProcessCount:              2,
		IndependentProcesses:              true,
		IdentityGatePassed:                true,
		ContaminationGatePassed:           true,
		CompletenessGatePassed:            true,
		DensityGatePassed:                 false,
		AggregateOnlyFullyGatedProcesses:  true,
		LowDensityRetainedDiagnostic:      true,
		SamplingCadenceVarianceRatio:      6.017391304,
		SamplingCadenceVarianceDisclosed:  true,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  false,
		CandidateSelected:                 false,
	}
	_ = evidence
	runMetalGPUProfilerStageSampleDensityGate()
}

func TestMetalGPUProfilerStageSampleDensityDerivedMismatch(t *testing.T) {
	evidence := sampleDensityEvidence{ // want `stage residual\+rmsnorm\[0\] density 0.5 disagrees with count/active-us 0.22`
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		GlobalEffectiveSampleCounts:       []float64{1384, 230},
		GlobalActiveMicroseconds:          []float64{1000, 1000},
		GlobalSamplesPerActiveMicrosecond: []float64{1.384, 0.23},
		GlobalMinimumSampleCount:          200,
		GlobalMinimumDensity:              0.2,
		StageNames:                        []string{"residual+rmsnorm", "q6_k"},
		StageEffectiveSampleCounts:        []float64{22, 66},
		StageActiveMicroseconds:           []float64{100, 100},
		StageSamplesPerActiveMicrosecond:  []float64{0.5, 0.66},
		StageMinimumSampleCounts:          []float64{10, 10},
		StageMinimumDensities:             []float64{0.1, 0.1},
		StageDensityStatuses:              []string{"sufficient", "sufficient"},
		AcceptedProcessIDs:                []string{"process-1", "process-2"},
		AcceptedProcessSampleDensities:    []float64{1.384, 0.23},
		RequiredAcceptedProcessCount:      2,
		AcceptedProcessCount:              2,
		IndependentProcesses:              true,
		IdentityGatePassed:                true,
		ContaminationGatePassed:           true,
		CompletenessGatePassed:            true,
		DensityGatePassed:                 true,
		AggregateOnlyFullyGatedProcesses:  true,
		LowDensityRetainedDiagnostic:      true,
		SamplingCadenceVarianceRatio:      6.017391304,
		SamplingCadenceVarianceDisclosed:  true,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  true,
		CandidateSelected:                 true,
	}
	_ = evidence
	runMetalGPUProfilerStageSampleDensityGate()
}

func TestMetalGPUProfilerStageSampleDensityCadenceMismatch(t *testing.T) {
	evidence := sampleDensityEvidence{ // want `sampling-cadence variance 2x disagrees with max/min density ratio 6.01739x`
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		GlobalEffectiveSampleCounts:       []float64{1384, 230},
		GlobalActiveMicroseconds:          []float64{1000, 1000},
		GlobalSamplesPerActiveMicrosecond: []float64{1.384, 0.23},
		GlobalMinimumSampleCount:          200,
		GlobalMinimumDensity:              0.2,
		StageNames:                        []string{"residual+rmsnorm", "q6_k"},
		StageEffectiveSampleCounts:        []float64{22, 66},
		StageActiveMicroseconds:           []float64{100, 100},
		StageSamplesPerActiveMicrosecond:  []float64{0.22, 0.66},
		StageMinimumSampleCounts:          []float64{10, 10},
		StageMinimumDensities:             []float64{0.1, 0.1},
		StageDensityStatuses:              []string{"sufficient", "sufficient"},
		AcceptedProcessIDs:                []string{"process-1", "process-2"},
		AcceptedProcessSampleDensities:    []float64{1.384, 0.23},
		RequiredAcceptedProcessCount:      2,
		AcceptedProcessCount:              2,
		IndependentProcesses:              true,
		IdentityGatePassed:                true,
		ContaminationGatePassed:           true,
		CompletenessGatePassed:            true,
		DensityGatePassed:                 true,
		AggregateOnlyFullyGatedProcesses:  true,
		LowDensityRetainedDiagnostic:      true,
		SamplingCadenceVarianceRatio:      2,
		SamplingCadenceVarianceDisclosed:  true,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  true,
		CandidateSelected:                 true,
	}
	_ = evidence
	runMetalGPUProfilerStageSampleDensityGate()
}

func TestMetalGPUProfilerStageSampleDensityUnderfilled(t *testing.T) {
	evidence := sampleDensityEvidence{ // want `medians are published before every density floor and accepted-process requirement passes; candidate is selected before every density floor and accepted-process requirement passes`
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		GlobalEffectiveSampleCounts:       []float64{1384},
		GlobalActiveMicroseconds:          []float64{1000},
		GlobalSamplesPerActiveMicrosecond: []float64{1.384},
		GlobalMinimumSampleCount:          200,
		GlobalMinimumDensity:              0.2,
		StageNames:                        []string{"residual+rmsnorm", "q6_k"},
		StageEffectiveSampleCounts:        []float64{22, 66},
		StageActiveMicroseconds:           []float64{100, 100},
		StageSamplesPerActiveMicrosecond:  []float64{0.22, 0.66},
		StageMinimumSampleCounts:          []float64{10, 10},
		StageMinimumDensities:             []float64{0.1, 0.1},
		StageDensityStatuses:              []string{"sufficient", "sufficient"},
		AcceptedProcessIDs:                []string{"process-1"},
		AcceptedProcessSampleDensities:    []float64{1.384},
		RequiredAcceptedProcessCount:      3,
		AcceptedProcessCount:              1,
		IndependentProcesses:              true,
		IdentityGatePassed:                true,
		ContaminationGatePassed:           true,
		CompletenessGatePassed:            true,
		DensityGatePassed:                 true,
		AggregateOnlyFullyGatedProcesses:  true,
		LowDensityRetainedDiagnostic:      true,
		SamplingCadenceVarianceRatio:      1,
		SamplingCadenceVarianceDisclosed:  true,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  true,
		CandidateSelected:                 true,
	}
	_ = evidence
	runMetalGPUProfilerStageSampleDensityGate()
}

func TestMetalGPUProfilerStageSampleDensityStable(t *testing.T) {
	evidence := sampleDensityEvidence{
		Hardware:                          "Apple M2 Pro",
		WorkloadDigest:                    "tinyllama-q4-k-m",
		GraphIdentity:                     "296-event-graph",
		GlobalEffectiveSampleCounts:       []float64{1384, 230},
		GlobalActiveMicroseconds:          []float64{1000, 1000},
		GlobalSamplesPerActiveMicrosecond: []float64{1.384, 0.23},
		GlobalMinimumSampleCount:          200,
		GlobalMinimumDensity:              0.2,
		StageNames:                        []string{"residual+rmsnorm", "q6_k"},
		StageEffectiveSampleCounts:        []float64{22, 66},
		StageActiveMicroseconds:           []float64{100, 100},
		StageSamplesPerActiveMicrosecond:  []float64{0.22, 0.66},
		StageMinimumSampleCounts:          []float64{10, 10},
		StageMinimumDensities:             []float64{0.1, 0.1},
		StageDensityStatuses:              []string{"sufficient", "sufficient"},
		AcceptedProcessIDs:                []string{"process-1", "process-2"},
		AcceptedProcessSampleDensities:    []float64{1.384, 0.23},
		RequiredAcceptedProcessCount:      2,
		AcceptedProcessCount:              2,
		IndependentProcesses:              true,
		IdentityGatePassed:                true,
		ContaminationGatePassed:           true,
		CompletenessGatePassed:            true,
		DensityGatePassed:                 true,
		AggregateOnlyFullyGatedProcesses:  true,
		LowDensityRetainedDiagnostic:      true,
		SamplingCadenceVarianceRatio:      6.017391304,
		SamplingCadenceVarianceDisclosed:  true,
		ExactOutputDigestPassed:           true,
		ExactEventTopologyPassed:          true,
		MediansPublished:                  true,
		CandidateSelected:                 true,
	}
	_ = evidence
	runMetalGPUProfilerStageSampleDensityGate()
}
