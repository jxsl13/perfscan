package ps6031

import "testing"

type counterPolicyEvidence struct {
	Hardware                      string
	WorkloadDigest                string
	GraphIdentity                 string
	ExactOutputDigestPassed       bool
	ExactEventTopologyPassed      bool
	CounterNames                  []string
	CounterPolicyClasses          []string
	CounterSampleCounts           []float64
	CounterAvailabilityStatuses   []string
	CounterObservedValues         []float64
	CounterContaminationCeilings  []float64
	CounterExpectedSamples        []float64
	CounterActiveStageDurations   []float64
	CounterMinimumExpectedSamples []float64
	CounterMinimumActiveDurations []float64
	CounterGateStatuses           []string
	CounterRejectionReasons       []string
	RequiredAcceptedRuns          int
	AttemptedRuns                 int
	AcceptedRuns                  int
	RejectedRuns                  int
	AttemptRejectionReasons       []string
	AllAttemptsDisclosed          bool
	RetrySelectionForbidden       bool
	MediansPublished              bool
	CandidateSelected             bool
}

func runMetalGPUProfilerCounterCompletenessPolicyClassification() {}

func TestMetalGPUProfilerCounterCompletenessPolicyMissing(t *testing.T) { // want `profiler counter-completeness harness has no semantic policy manifest`
	runMetalGPUProfilerCounterCompletenessPolicyClassification()
}

func TestMetalGPUProfilerCounterCompletenessPolicyIncomplete(t *testing.T) {
	evidence := counterPolicyEvidence{ // want `profiler counter-policy evidence is incomplete; missing workload digest, graph identity, exact output/digest status`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUProfilerCounterCompletenessPolicyClassification()
}

func TestMetalGPUProfilerCounterCompletenessPolicyWrongClasses(t *testing.T) {
	evidence := counterPolicyEvidence{ // want `required-decision counter "decision" is unsampled but not rejected; required-decision counter "decision" has no rejection reason; contamination-sentinel counter "occupancy" is missing/above-ceiling but not rejected`
		Hardware:                      "Apple M2 Pro",
		WorkloadDigest:                "tinyllama-q4-k-m",
		GraphIdentity:                 "296-event-graph",
		ExactOutputDigestPassed:       true,
		ExactEventTopologyPassed:      true,
		CounterNames:                  []string{"decision", "occupancy", "capability", "short-stage", "diagnostic"},
		CounterPolicyClasses:          []string{"required-decision", "contamination-sentinel", "capability", "sparse-stage", "optional-diagnostic"},
		CounterSampleCounts:           []float64{0, 0, 0, 0, 0},
		CounterAvailabilityStatuses:   []string{"available", "available", "available", "available", "available"},
		CounterObservedValues:         []float64{0, 0, 0, 0, 0},
		CounterContaminationCeilings:  []float64{0, 0.01, 0, 0, 0},
		CounterExpectedSamples:        []float64{10, 10, 0, 0.5, 0},
		CounterActiveStageDurations:   []float64{100, 100, 0, 2, 0},
		CounterMinimumExpectedSamples: []float64{1, 1, 0, 1, 0},
		CounterMinimumActiveDurations: []float64{5, 5, 0, 5, 0},
		CounterGateStatuses:           []string{"accepted", "accepted", "rejected", "rejected", "rejected"},
		CounterRejectionReasons:       []string{"", "", "unsupported", "below resolution", "optional missing"},
		RequiredAcceptedRuns:          1,
		AttemptedRuns:                 1,
		AcceptedRuns:                  1,
		RejectedRuns:                  0,
		AttemptRejectionReasons:       []string{},
		AllAttemptsDisclosed:          true,
		RetrySelectionForbidden:       true,
		MediansPublished:              true,
		CandidateSelected:             true,
	}
	_ = evidence
	runMetalGPUProfilerCounterCompletenessPolicyClassification()
}

func TestMetalGPUProfilerCounterCompletenessPolicyPrematurePublication(t *testing.T) {
	evidence := counterPolicyEvidence{ // want `medians are published before the predeclared accepted-run count; candidate is selected before the predeclared accepted-run count`
		Hardware:                      "Apple M2 Pro",
		WorkloadDigest:                "tinyllama-q4-k-m",
		GraphIdentity:                 "296-event-graph",
		ExactOutputDigestPassed:       true,
		ExactEventTopologyPassed:      true,
		CounterNames:                  []string{"decision", "occupancy", "capability", "short-stage", "diagnostic"},
		CounterPolicyClasses:          []string{"required-decision", "contamination-sentinel", "capability", "sparse-stage", "optional-diagnostic"},
		CounterSampleCounts:           []float64{1354, 1354, 0, 0, 0},
		CounterAvailabilityStatuses:   []string{"available", "available", "unsupported", "available", "available"},
		CounterObservedValues:         []float64{1, 0, 0, 0, 0},
		CounterContaminationCeilings:  []float64{0, 0.01, 0, 0, 0},
		CounterExpectedSamples:        []float64{1354, 1354, 0, 0.5, 0},
		CounterActiveStageDurations:   []float64{100, 100, 0, 2, 0},
		CounterMinimumExpectedSamples: []float64{1, 1, 0, 1, 0},
		CounterMinimumActiveDurations: []float64{5, 5, 0, 5, 0},
		CounterGateStatuses:           []string{"accepted", "accepted", "unsupported", "below-resolution-diagnostic", "missing-diagnostic"},
		CounterRejectionReasons:       []string{"", "", "", "", ""},
		RequiredAcceptedRuns:          3,
		AttemptedRuns:                 5,
		AcceptedRuns:                  1,
		RejectedRuns:                  4,
		AttemptRejectionReasons:       []string{"Partial Renders Count", "Store Limiter", "Buffer Load", "rmsnorm ALU"},
		AllAttemptsDisclosed:          true,
		RetrySelectionForbidden:       true,
		MediansPublished:              true,
		CandidateSelected:             true,
	}
	_ = evidence
	runMetalGPUProfilerCounterCompletenessPolicyClassification()
}

func TestMetalGPUProfilerCounterCompletenessPolicyStable(t *testing.T) {
	evidence := counterPolicyEvidence{
		Hardware:                      "Apple M2 Pro",
		WorkloadDigest:                "tinyllama-q4-k-m",
		GraphIdentity:                 "296-event-graph",
		ExactOutputDigestPassed:       true,
		ExactEventTopologyPassed:      true,
		CounterNames:                  []string{"decision", "occupancy", "capability", "short-stage", "diagnostic"},
		CounterPolicyClasses:          []string{"required-decision", "contamination-sentinel", "capability", "sparse-stage", "optional-diagnostic"},
		CounterSampleCounts:           []float64{1354, 1354, 0, 0, 0},
		CounterAvailabilityStatuses:   []string{"available", "available", "unsupported", "available", "available"},
		CounterObservedValues:         []float64{1, 0, 0, 0, 0},
		CounterContaminationCeilings:  []float64{0, 0.01, 0, 0, 0},
		CounterExpectedSamples:        []float64{1354, 1354, 0, 0.5, 0},
		CounterActiveStageDurations:   []float64{100, 100, 0, 2, 0},
		CounterMinimumExpectedSamples: []float64{1, 1, 0, 1, 0},
		CounterMinimumActiveDurations: []float64{5, 5, 0, 5, 0},
		CounterGateStatuses:           []string{"accepted", "accepted", "unsupported", "below-resolution-diagnostic", "missing-diagnostic"},
		CounterRejectionReasons:       []string{"", "", "", "", ""},
		RequiredAcceptedRuns:          3,
		AttemptedRuns:                 5,
		AcceptedRuns:                  1,
		RejectedRuns:                  4,
		AttemptRejectionReasons:       []string{"Partial Renders Count", "Store Limiter", "Buffer Load", "rmsnorm ALU"},
		AllAttemptsDisclosed:          true,
		RetrySelectionForbidden:       true,
		MediansPublished:              false,
		CandidateSelected:             false,
	}
	_ = evidence
	runMetalGPUProfilerCounterCompletenessPolicyClassification()
}
