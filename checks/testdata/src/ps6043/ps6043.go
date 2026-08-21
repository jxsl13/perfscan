package ps6043

import "testing"

type submissionBoundaryEvidence struct {
	Hardware                               string
	ControlBenchmarkIdentityRevision       string
	CandidateBenchmarkIdentityRevision     string
	WorkloadShape                          string
	ControlOperationsPerSubmission         int
	CandidateOperationsPerSubmission       int
	ControlSubmissionsPerTimedSample       int
	CandidateSubmissionsPerTimedSample     int
	ControlWaitsPerTimedSample             int
	CandidateWaitsPerTimedSample           int
	ControlAmortizedLatencyUS              float64
	CandidateAmortizedLatencyUS            float64
	ControlSubmissionLatencyUS             float64
	CandidateSubmissionLatencyUS           float64
	ControlResidentWorkingSetBytes         int
	CandidateResidentWorkingSetBytes       int
	ControlWeightReuseMode                 string
	CandidateWeightReuseMode               string
	ControlWarmupCompilationBoundary       string
	CandidateWarmupCompilationBoundary     string
	ControlIndependentProcessLatenciesUS   []float64
	CandidateIndependentProcessLatenciesUS []float64
	BoundaryCompatible                     bool
	SpeedupComputed                        bool
	ComparisonClassification               string
	FinalDecision                          string
}

func runGPULeafSubmissionBoundaryWorkingSetReuse() {}

func TestGPULeafSubmissionBoundaryWorkingSetReuseMissing(t *testing.T) { // want `accelerator leaf comparison has no submission-boundary manifest`
	runGPULeafSubmissionBoundaryWorkingSetReuse()
}

func TestGPULeafSubmissionBoundaryWorkingSetReuseIncomplete(t *testing.T) {
	evidence := submissionBoundaryEvidence{ // want `accelerator submission-boundary evidence is incomplete; missing control benchmark identity/revision, candidate benchmark identity/revision, workload shape`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runGPULeafSubmissionBoundaryWorkingSetReuse()
}

func TestGPULeafSubmissionBoundaryWorkingSetReuseUnsafeComparison(t *testing.T) {
	evidence := submissionBoundaryEvidence{ // want `boundary compatibility is true but operations/reuse/working-set/warmup metadata computes false; speedup is computed across mismatched submission or working-set boundaries; comparison classification "performance-win" does not label the boundary mismatch; final decision "retained" accepts a boundary-mismatched cross-system comparison`
		Hardware:                               "Apple M2 Pro",
		ControlBenchmarkIdentityRevision:       "llama.cpp test-backend-ops 4df29be4",
		CandidateBenchmarkIdentityRevision:     "GoAI diagnostic main",
		WorkloadShape:                          "M=1 Q4_K 2048x2048 matvec",
		ControlOperationsPerSubmission:         65_536,
		CandidateOperationsPerSubmission:       1,
		ControlSubmissionsPerTimedSample:       1,
		CandidateSubmissionsPerTimedSample:     1,
		ControlWaitsPerTimedSample:             1,
		CandidateWaitsPerTimedSample:           1,
		ControlAmortizedLatencyUS:              15.88,
		CandidateAmortizedLatencyUS:            160.7335,
		ControlSubmissionLatencyUS:             15.88 * 65_536,
		CandidateSubmissionLatencyUS:           160.7335,
		ControlResidentWorkingSetBytes:         2_359_296,
		CandidateResidentWorkingSetBytes:       2_359_296,
		ControlWeightReuseMode:                 "same weight reused 65536 times",
		CandidateWeightReuseMode:               "one operation per submit",
		ControlWarmupCompilationBoundary:       "compiled and warm before timing",
		CandidateWarmupCompilationBoundary:     "compiled and warm before timing",
		ControlIndependentProcessLatenciesUS:   []float64{15.81, 15.88, 15.93},
		CandidateIndependentProcessLatenciesUS: []float64{159.9, 160.7335, 161.2},
		BoundaryCompatible:                     true,
		SpeedupComputed:                        true,
		ComparisonClassification:               "performance-win",
		FinalDecision:                          "retained",
	}
	_ = evidence
	runGPULeafSubmissionBoundaryWorkingSetReuse()
}

func TestGPULeafSubmissionBoundaryWorkingSetReuseStaleMetrics(t *testing.T) {
	evidence := submissionBoundaryEvidence{ // want `control submission latency 10 disagrees with amortized\*operations 4; control independent-process distribution has fewer than three samples; candidate independent-process distribution contains a non-positive latency`
		Hardware:                               "Apple M2 Pro",
		ControlBenchmarkIdentityRevision:       "control diagnostic",
		CandidateBenchmarkIdentityRevision:     "candidate diagnostic",
		WorkloadShape:                          "M=1 Q4_K 2048x2048 matvec",
		ControlOperationsPerSubmission:         2,
		CandidateOperationsPerSubmission:       1,
		ControlSubmissionsPerTimedSample:       1,
		CandidateSubmissionsPerTimedSample:     1,
		ControlWaitsPerTimedSample:             1,
		CandidateWaitsPerTimedSample:           1,
		ControlAmortizedLatencyUS:              2,
		CandidateAmortizedLatencyUS:            4,
		ControlSubmissionLatencyUS:             10,
		CandidateSubmissionLatencyUS:           4,
		ControlResidentWorkingSetBytes:         2_359_296,
		CandidateResidentWorkingSetBytes:       2_359_296,
		ControlWeightReuseMode:                 "same weight reused",
		CandidateWeightReuseMode:               "one operation per submit",
		ControlWarmupCompilationBoundary:       "compiled and warm before timing",
		CandidateWarmupCompilationBoundary:     "compiled and warm before timing",
		ControlIndependentProcessLatenciesUS:   []float64{2, 2.1},
		CandidateIndependentProcessLatenciesUS: []float64{4, 0, 4.1},
		BoundaryCompatible:                     false,
		SpeedupComputed:                        false,
		ComparisonClassification:               "boundary-mismatched",
		FinalDecision:                          "rejected",
	}
	_ = evidence
	runGPULeafSubmissionBoundaryWorkingSetReuse()
}

func TestGPULeafSubmissionBoundaryWorkingSetReuseStable(t *testing.T) {
	evidence := submissionBoundaryEvidence{
		Hardware:                               "Apple M2 Pro",
		ControlBenchmarkIdentityRevision:       "llama.cpp test-backend-ops 4df29be4",
		CandidateBenchmarkIdentityRevision:     "GoAI diagnostic main",
		WorkloadShape:                          "M=1 Q4_K 2048x2048 matvec",
		ControlOperationsPerSubmission:         65_536,
		CandidateOperationsPerSubmission:       1,
		ControlSubmissionsPerTimedSample:       1,
		CandidateSubmissionsPerTimedSample:     1,
		ControlWaitsPerTimedSample:             1,
		CandidateWaitsPerTimedSample:           1,
		ControlAmortizedLatencyUS:              15.88,
		CandidateAmortizedLatencyUS:            160.7335,
		ControlSubmissionLatencyUS:             15.88 * 65_536,
		CandidateSubmissionLatencyUS:           160.7335,
		ControlResidentWorkingSetBytes:         2_359_296,
		CandidateResidentWorkingSetBytes:       2_359_296,
		ControlWeightReuseMode:                 "same weight reused 65536 times",
		CandidateWeightReuseMode:               "one operation per submit",
		ControlWarmupCompilationBoundary:       "compiled and warm before timing",
		CandidateWarmupCompilationBoundary:     "compiled and warm before timing",
		ControlIndependentProcessLatenciesUS:   []float64{15.81, 15.88, 15.93},
		CandidateIndependentProcessLatenciesUS: []float64{159.9, 160.7335, 161.2},
		BoundaryCompatible:                     false,
		SpeedupComputed:                        false,
		ComparisonClassification:               "boundary-mismatched",
		FinalDecision:                          "rejected",
	}
	_ = evidence
	runGPULeafSubmissionBoundaryWorkingSetReuse()
}
