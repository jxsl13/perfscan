package ps6026

import "testing"

type activeExtentEvidence struct {
	Hardware                           string
	Workload                           string
	ExtentUnit                         string
	AllocatedElements                  int
	LogicalActiveElements              int
	ControlDispatchedElements          int
	CandidateDispatchedElements        int
	CorrectedUnfusedDispatchedElements int
	DispatchAmplificationRatio         float64
	OperationShapeLabels               map[string]string
	ExactShapeCampaign                 bool
	PaddingJustified                   bool
	JSONPreservesBufferCapacity        bool
	JSONPreservesActiveShape           bool
	CorrectedUnfusedNS                 float64
	FusionCandidateNS                  float64
	CorrectedControlCandidateRatio     float64
	ExactParityPassed                  bool
	ControlLogicalActiveElements       int
	CandidateLogicalActiveElements     int
}

func dispatchGPUActiveExtentControlCandidate() {}

func BenchmarkMetalGPUDispatchActiveExtentControlCandidateMissing(b *testing.B) { // want `GPU active-extent harness has no dispatch-work manifest; missing hardware, workload, common extent unit`
	for range b.N {
		dispatchGPUActiveExtentControlCandidate()
	}
}

func BenchmarkMetalGPUDispatchActiveExtentControlCandidateIncomplete(b *testing.B) {
	evidence := activeExtentEvidence{ // want `GPU dispatch-work evidence is incomplete; missing common extent unit, allocated buffer capacity, logical active extent, control dispatched extent`
		Hardware: "Apple M2 Pro",
		Workload: "TinyLlama rows=1 decode",
	}
	_ = evidence
	for range b.N {
		dispatchGPUActiveExtentControlCandidate()
	}
}

func BenchmarkMetalGPUDispatchActiveExtentControlCandidateAmplified(b *testing.B) {
	evidence := activeExtentEvidence{ // want `GPU active-extent audit: control dispatch 4.194e\+06 exceeds exact logical extent 4096 without justified padding; before/after arms execute different dispatched work \(control 4.194e\+06, candidate 4096\); attribute no fusion win until using the corrected-unfused control; fusion candidate is slower than corrected same-work control \(control/candidate 0.984916x\); block fusion attribution`
		Hardware:                           "Apple M2 Pro",
		Workload:                           "TinyLlama rows=1 decode",
		ExtentUnit:                         "elements",
		AllocatedElements:                  4_194_304,
		LogicalActiveElements:              4096,
		ControlDispatchedElements:          4_194_304,
		CandidateDispatchedElements:        4096,
		CorrectedUnfusedDispatchedElements: 4096,
		DispatchAmplificationRatio:         1024,
		OperationShapeLabels:               map[string]string{"binary": "rows=1, hidden=4096"},
		ExactShapeCampaign:                 true,
		PaddingJustified:                   false,
		JSONPreservesBufferCapacity:        true,
		JSONPreservesActiveShape:           true,
		CorrectedUnfusedNS:                 1_190_455_583,
		FusionCandidateNS:                  1_208_686_833,
		CorrectedControlCandidateRatio:     0.984916,
		ExactParityPassed:                  true,
	}
	_ = evidence
	for range b.N {
		dispatchGPUActiveExtentControlCandidate()
	}
}

func BenchmarkMetalGPUDispatchActiveExtentControlCandidateRatioMismatch(b *testing.B) {
	evidence := activeExtentEvidence{ // want `GPU active-extent audit: recorded dispatch amplification 3x disagrees with control-dispatched/active ratio 2x`
		Hardware:                           "Apple M2 Pro",
		Workload:                           "padded matvec",
		ExtentUnit:                         "elements",
		AllocatedElements:                  8192,
		LogicalActiveElements:              4096,
		ControlDispatchedElements:          8192,
		CandidateDispatchedElements:        8192,
		CorrectedUnfusedDispatchedElements: 8192,
		DispatchAmplificationRatio:         3,
		OperationShapeLabels:               map[string]string{"matvec": "logical=4096,padded=8192"},
		ExactShapeCampaign:                 false,
		PaddingJustified:                   true,
		JSONPreservesBufferCapacity:        true,
		JSONPreservesActiveShape:           true,
		CorrectedUnfusedNS:                 100,
		FusionCandidateNS:                  90,
		CorrectedControlCandidateRatio:     1.111111,
		ExactParityPassed:                  true,
	}
	_ = evidence
	for range b.N {
		dispatchGPUActiveExtentControlCandidate()
	}
}

func BenchmarkMetalGPUDispatchActiveExtentControlCandidateLogicalMismatch(b *testing.B) {
	evidence := activeExtentEvidence{ // want `GPU active-extent audit: control/candidate logical active extents differ \(4096 vs 2048\)`
		Hardware:                           "Apple M2 Pro",
		Workload:                           "matvec",
		ExtentUnit:                         "elements",
		AllocatedElements:                  4096,
		LogicalActiveElements:              4096,
		ControlDispatchedElements:          4096,
		CandidateDispatchedElements:        4096,
		CorrectedUnfusedDispatchedElements: 4096,
		DispatchAmplificationRatio:         1,
		OperationShapeLabels:               map[string]string{"matvec": "hidden=4096"},
		ExactShapeCampaign:                 true,
		PaddingJustified:                   false,
		JSONPreservesBufferCapacity:        true,
		JSONPreservesActiveShape:           true,
		CorrectedUnfusedNS:                 100,
		FusionCandidateNS:                  90,
		CorrectedControlCandidateRatio:     1.111111,
		ExactParityPassed:                  true,
		ControlLogicalActiveElements:       4096,
		CandidateLogicalActiveElements:     2048,
	}
	_ = evidence
	for range b.N {
		dispatchGPUActiveExtentControlCandidate()
	}
}

func BenchmarkMetalGPUDispatchActiveExtentControlCandidateStable(b *testing.B) {
	evidence := activeExtentEvidence{
		Hardware:                           "Apple M2 Pro",
		Workload:                           "padded matvec",
		ExtentUnit:                         "elements",
		AllocatedElements:                  8192,
		LogicalActiveElements:              4096,
		ControlDispatchedElements:          8192,
		CandidateDispatchedElements:        8192,
		CorrectedUnfusedDispatchedElements: 8192,
		DispatchAmplificationRatio:         2,
		OperationShapeLabels:               map[string]string{"matvec": "logical=4096,padded=8192"},
		ExactShapeCampaign:                 false,
		PaddingJustified:                   true,
		JSONPreservesBufferCapacity:        true,
		JSONPreservesActiveShape:           true,
		CorrectedUnfusedNS:                 100,
		FusionCandidateNS:                  90,
		CorrectedControlCandidateRatio:     1.111111,
		ExactParityPassed:                  true,
	}
	_ = evidence
	for range b.N {
		dispatchGPUActiveExtentControlCandidate()
	}
}
