package ps6007

import (
	"testing"
	"time"
)

func mpsGEMM() {}

const (
	removablePassShare     = 0.04
	residencyPromotionGate = 1.10
)

func BenchmarkMetalFFNResidency(b *testing.B) {
	for range b.N {
		mpsGEMM()
	}
	_ = removablePassShare
	_ = residencyPromotionGate // want `declared removable-pass share 4\.0000% has a zero-cost whole-chain ceiling of 1\.0417x, below the 1\.1000x promotion gate`
}

const (
	baselineMicros             = 1000.0
	removableConversionMicros  = 25.0
	removableElementwiseMicros = 15.0
	conversionSpeedupGate      = 1.10
)

func BenchmarkCUDAGEMMConversions(b *testing.B) {
	for range b.N {
		mpsGEMM()
	}
	_, _, _ = baselineMicros, removableConversionMicros, removableElementwiseMicros
	_ = conversionSpeedupGate // want `declared removable-pass share 4\.0000% has a zero-cost whole-chain ceiling of 1\.0417x, below the 1\.1000x promotion gate`
}

const (
	baselineDuration          time.Duration = 1000 * time.Microsecond
	removableTransferDuration time.Duration = 40 * time.Microsecond
	durationPromotionGate                   = 1.10
)

func BenchmarkMPSMatmulDuration(b *testing.B) {
	for range b.N {
		mpsGEMM()
	}
	_, _ = baselineDuration, removableTransferDuration
	_ = durationPromotionGate // want `declared removable-pass share 4\.0000% has a zero-cost whole-chain ceiling of 1\.0417x, below the 1\.1000x promotion gate`
}

const (
	removableConversionPercent  = 2.5
	removableElementwisePercent = 1.5
	percentTargetSpeedup        = 1.10
)

func BenchmarkGPUMatmulPercent(b *testing.B) {
	for range b.N {
		mpsGEMM()
	}
	_, _ = removableConversionPercent, removableElementwisePercent
	_ = percentTargetSpeedup // want `declared removable-pass share 4\.0000% has a zero-cost whole-chain ceiling of 1\.0417x, below the 1\.1000x promotion gate`
}

// A 10% removable share can theoretically meet a 1.10x gate.
const (
	reachableRemovableShare = 0.10
	reachablePromotionGate  = 1.10
)

func BenchmarkMetalGEMMReachable(b *testing.B) {
	for range b.N {
		mpsGEMM()
	}
	_, _ = reachableRemovableShare, reachablePromotionGate
}

// Runtime measurements are not compile-time ceiling evidence.
func BenchmarkMetalGEMMRuntime(b *testing.B) {
	baselineMicros := float64(b.N)
	removableMicros := baselineMicros * 0.04
	promotionGate := 1.10
	_, _, _ = baselineMicros, removableMicros, promotionGate
}

// Mixed implicit units are rejected instead of inventing a conversion.
const (
	mixedBaselineMicros = 1000.0
	mixedRemovableNanos = 40.0
	mixedPromotionGate  = 1.10
)

func BenchmarkMetalGEMMMixedUnits(b *testing.B) {
	for range b.N {
		mpsGEMM()
	}
	_, _, _ = mixedBaselineMicros, mixedRemovableNanos, mixedPromotionGate
}

// No accelerator/GEMM context: ordinary component timings remain silent.
const (
	cpuRemovableShare = 0.01
	cpuPromotionGate  = 1.10
)

func BenchmarkParser(b *testing.B) {
	for range b.N {
	}
	_, _ = cpuRemovableShare, cpuPromotionGate
}
