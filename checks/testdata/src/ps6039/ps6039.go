package ps6039

import "testing"

type multiResourceArenaComparison struct {
	Hardware                         string
	WorkloadIdentity                 string
	ControlResourceCount             int
	CandidateResourceCount           int
	ControlResidentBytes             float64
	CandidateResidentBytes           float64
	CacheCapacityBytes               float64
	WorkingSetExceedsCache           bool
	ControlStorageMode               string
	CandidateStorageMode             string
	OffsetAlignmentBytes             int
	NonzeroOffsetCovered             bool
	ControlPersistentBytes           float64
	CandidatePersistentBytes         float64
	ControlTransientBytes            float64
	CandidateTransientBytes          float64
	IdenticalInputBytes              bool
	IdenticalKernels                 bool
	IdenticalDispatchOrderCount      bool
	IdenticalHazards                 bool
	IdenticalCommandBufferCount      bool
	WarmStreamingPassed              bool
	IterationCount                   int
	ControlLatencyNS                 float64
	CandidateLatencyNS               float64
	ControlCandidateRatio            float64
	ControlEffectiveBytesPerSecond   float64
	CandidateEffectiveBytesPerSecond float64
	ControlAllocationBytes           int
	CandidateAllocationBytes         int
	ControlAllocationCount           int
	CandidateAllocationCount         int
	NonzeroOffsetParityPassed        bool
	ArenaViewLifetimePassed          bool
	FinalDecision                    string
}

func runMetalGPUMultiResourceArenaCacheExceedingComparison() {}

func TestMetalGPUMultiResourceArenaCacheExceedingMissing(t *testing.T) { // want `GPU multi-resource arena campaign has no cache-exceeding manifest`
	runMetalGPUMultiResourceArenaCacheExceedingComparison()
}

func TestMetalGPUMultiResourceArenaCacheExceedingIncomplete(t *testing.T) {
	evidence := multiResourceArenaComparison{ // want `GPU arena comparison evidence is incomplete; missing workload identity, control resource count, candidate resource count`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
	runMetalGPUMultiResourceArenaCacheExceedingComparison()
}

func TestMetalGPUMultiResourceArenaCacheExceedingRetainedSlower(t *testing.T) {
	evidence := multiResourceArenaComparison{ // want `final decision "retained" retains a slower arena candidate \(control/candidate 0.985361x\)`
		Hardware:                         "Apple M2 Pro",
		WorkloadIdentity:                 "44 Q4_K K2048,N5632 weights",
		ControlResourceCount:             44,
		CandidateResourceCount:           1,
		ControlResidentBytes:             285_000_000,
		CandidateResidentBytes:           285_000_000,
		CacheCapacityBytes:               128_000_000,
		WorkingSetExceedsCache:           true,
		ControlStorageMode:               "shared",
		CandidateStorageMode:             "shared",
		OffsetAlignmentBytes:             256,
		NonzeroOffsetCovered:             true,
		ControlPersistentBytes:           285_000_000,
		CandidatePersistentBytes:         285_000_000,
		ControlTransientBytes:            0,
		CandidateTransientBytes:          0,
		IdenticalInputBytes:              true,
		IdenticalKernels:                 true,
		IdenticalDispatchOrderCount:      true,
		IdenticalHazards:                 true,
		IdenticalCommandBufferCount:      true,
		WarmStreamingPassed:              true,
		IterationCount:                   20,
		ControlLatencyNS:                 2_124_454,
		CandidateLatencyNS:               2_156_015,
		ControlCandidateRatio:            2_124_454.0 / 2_156_015,
		ControlEffectiveBytesPerSecond:   285_000_000.0 * 1e9 / 2_124_454,
		CandidateEffectiveBytesPerSecond: 285_000_000.0 * 1e9 / 2_156_015,
		ControlAllocationBytes:           8,
		CandidateAllocationBytes:         8,
		ControlAllocationCount:           1,
		CandidateAllocationCount:         1,
		NonzeroOffsetParityPassed:        true,
		ArenaViewLifetimePassed:          true,
		FinalDecision:                    "retained",
	}
	_ = evidence
	runMetalGPUMultiResourceArenaCacheExceedingComparison()
}

func TestMetalGPUMultiResourceArenaCacheExceedingUnequal(t *testing.T) {
	evidence := multiResourceArenaComparison{ // want `control/candidate resident bytes differ \(2.85e\+08 vs 2e\+08\); working set 2.85e\+08 bytes does not exceed 3e\+08-byte cache capacity; control/candidate persistentbytes differ`
		Hardware:                         "Apple M2 Pro",
		WorkloadIdentity:                 "44 Q4_K K2048,N5632 weights",
		ControlResourceCount:             44,
		CandidateResourceCount:           1,
		ControlResidentBytes:             285_000_000,
		CandidateResidentBytes:           200_000_000,
		CacheCapacityBytes:               300_000_000,
		WorkingSetExceedsCache:           false,
		ControlStorageMode:               "shared",
		CandidateStorageMode:             "private",
		OffsetAlignmentBytes:             256,
		NonzeroOffsetCovered:             true,
		ControlPersistentBytes:           285_000_000,
		CandidatePersistentBytes:         200_000_000,
		ControlTransientBytes:            0,
		CandidateTransientBytes:          0,
		IdenticalInputBytes:              true,
		IdenticalKernels:                 true,
		IdenticalDispatchOrderCount:      true,
		IdenticalHazards:                 true,
		IdenticalCommandBufferCount:      true,
		WarmStreamingPassed:              true,
		IterationCount:                   20,
		ControlLatencyNS:                 2_124_454,
		CandidateLatencyNS:               2_000_000,
		ControlCandidateRatio:            2_124_454.0 / 2_000_000,
		ControlEffectiveBytesPerSecond:   285_000_000.0 * 1e9 / 2_124_454,
		CandidateEffectiveBytesPerSecond: 200_000_000.0 * 1e9 / 2_000_000,
		ControlAllocationBytes:           8,
		CandidateAllocationBytes:         8,
		ControlAllocationCount:           1,
		CandidateAllocationCount:         1,
		NonzeroOffsetParityPassed:        true,
		ArenaViewLifetimePassed:          true,
		FinalDecision:                    "removed",
	}
	_ = evidence
	runMetalGPUMultiResourceArenaCacheExceedingComparison()
}

func TestMetalGPUMultiResourceArenaCacheExceedingRatioMismatch(t *testing.T) {
	evidence := multiResourceArenaComparison{ // want `control/candidate ratio 1.2x disagrees with latency ratio 0.985361x`
		Hardware:                         "Apple M2 Pro",
		WorkloadIdentity:                 "44 Q4_K K2048,N5632 weights",
		ControlResourceCount:             44,
		CandidateResourceCount:           1,
		ControlResidentBytes:             285_000_000,
		CandidateResidentBytes:           285_000_000,
		CacheCapacityBytes:               128_000_000,
		WorkingSetExceedsCache:           true,
		ControlStorageMode:               "shared",
		CandidateStorageMode:             "shared",
		OffsetAlignmentBytes:             256,
		NonzeroOffsetCovered:             true,
		ControlPersistentBytes:           285_000_000,
		CandidatePersistentBytes:         285_000_000,
		ControlTransientBytes:            0,
		CandidateTransientBytes:          0,
		IdenticalInputBytes:              true,
		IdenticalKernels:                 true,
		IdenticalDispatchOrderCount:      true,
		IdenticalHazards:                 true,
		IdenticalCommandBufferCount:      true,
		WarmStreamingPassed:              true,
		IterationCount:                   20,
		ControlLatencyNS:                 2_124_454,
		CandidateLatencyNS:               2_156_015,
		ControlCandidateRatio:            1.2,
		ControlEffectiveBytesPerSecond:   285_000_000.0 * 1e9 / 2_124_454,
		CandidateEffectiveBytesPerSecond: 285_000_000.0 * 1e9 / 2_156_015,
		ControlAllocationBytes:           8,
		CandidateAllocationBytes:         8,
		ControlAllocationCount:           1,
		CandidateAllocationCount:         1,
		NonzeroOffsetParityPassed:        true,
		ArenaViewLifetimePassed:          true,
		FinalDecision:                    "removed",
	}
	_ = evidence
	runMetalGPUMultiResourceArenaCacheExceedingComparison()
}

func TestMetalGPUMultiResourceArenaCacheExceedingStable(t *testing.T) {
	evidence := multiResourceArenaComparison{
		Hardware:                         "Apple M2 Pro",
		WorkloadIdentity:                 "44 Q4_K K2048,N5632 weights",
		ControlResourceCount:             44,
		CandidateResourceCount:           1,
		ControlResidentBytes:             285_000_000,
		CandidateResidentBytes:           285_000_000,
		CacheCapacityBytes:               128_000_000,
		WorkingSetExceedsCache:           true,
		ControlStorageMode:               "shared",
		CandidateStorageMode:             "shared",
		OffsetAlignmentBytes:             256,
		NonzeroOffsetCovered:             true,
		ControlPersistentBytes:           285_000_000,
		CandidatePersistentBytes:         285_000_000,
		ControlTransientBytes:            0,
		CandidateTransientBytes:          0,
		IdenticalInputBytes:              true,
		IdenticalKernels:                 true,
		IdenticalDispatchOrderCount:      true,
		IdenticalHazards:                 true,
		IdenticalCommandBufferCount:      true,
		WarmStreamingPassed:              true,
		IterationCount:                   20,
		ControlLatencyNS:                 2_124_454,
		CandidateLatencyNS:               2_156_015,
		ControlCandidateRatio:            2_124_454.0 / 2_156_015,
		ControlEffectiveBytesPerSecond:   285_000_000.0 * 1e9 / 2_124_454,
		CandidateEffectiveBytesPerSecond: 285_000_000.0 * 1e9 / 2_156_015,
		ControlAllocationBytes:           8,
		CandidateAllocationBytes:         8,
		ControlAllocationCount:           1,
		CandidateAllocationCount:         1,
		NonzeroOffsetParityPassed:        true,
		ArenaViewLifetimePassed:          true,
		FinalDecision:                    "removed",
	}
	_ = evidence
	runMetalGPUMultiResourceArenaCacheExceedingComparison()
}
