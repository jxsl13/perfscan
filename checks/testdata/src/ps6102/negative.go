package ps6102

import (
	"testing"
	"time"
)

func TestEarlySkip(t *testing.T) {
	if testing.Short() {
		t.Skip("performance test")
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestEarlyReturn(t *testing.T) {
	short := testing.Short()
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestInverseBranch(t *testing.T) {
	if !testing.Short() {
		latency := time.Since(time.Now())
		if latency > time.Second {
			t.Fatal("slow")
		}
	}
}

func TestInverseElse(t *testing.T) {
	if testing.Short() {
		work()
	} else {
		throughput := 1.0
		if throughput < 2 {
			t.Fatal("slow")
		}
	}
}

func TestUnknownOrGuard(t *testing.T) {
	if testing.Short() || unknown() {
		t.Skip("all short-mode paths")
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestConstantGuard(t *testing.T) {
	if testing.Short() && true {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestImmutableAliasComparedToTrue(t *testing.T) {
	short := testing.Short()
	if short == true {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestShortCircuitThreshold(t *testing.T) {
	latency := time.Since(time.Now())
	if testing.Short() || latency > time.Second {
		t.Fatal("threshold is not evaluated in short mode")
	}
}

func TestThresholdCannotAffectConstantCondition(t *testing.T) {
	latency := time.Since(time.Now())
	if latency > time.Second && false {
		t.Log("unreachable")
	} else {
		t.Fatal("failure is independent of latency")
	}
}

func TestLogOnlyMeasurement(t *testing.T) {
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Logf("slow: %v", latency)
	}
}

func TestCorrectnessDuration(t *testing.T) {
	got := time.Hour
	if got != time.Minute {
		t.Error("functional duration mismatch")
	}
}

func TestUnusedClosure(t *testing.T) {
	unused := func(speedup float64) {
		if speedup < 1.2 {
			t.Fatal("unused")
		}
	}
	_ = unused
}

func skipPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("performance test")
	}
}

func TestCalledSkipHelper(t *testing.T) {
	skipPerformance(t)
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestKnownShortSwitch(t *testing.T) {
	switch testing.Short() {
	case true:
		return
	default:
		latency := time.Since(time.Now())
		if latency > time.Second {
			t.Fatal("slow")
		}
	}
}

func TestDefinitelyEmptyRange(t *testing.T) {
	for range [0]int{} {
		latency := time.Since(time.Now())
		if latency > time.Second {
			t.Fatal("slow")
		}
	}
}

func TestTerminatingLoop(t *testing.T) {
	for {
		t.Skip("short-mode path never leaves the loop")
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func constantResult(time.Duration) int { return 1 }

func TestMetricArgumentDoesNotTaintOpaqueResult(t *testing.T) {
	elapsed := time.Since(time.Now())
	got := constantResult(elapsed)
	if got != 1 {
		t.Fatal("functional result")
	}
}

func observeBool(*bool) {}

func TestReadOnlyPointerHelperPreservesAlias(t *testing.T) {
	short := testing.Short()
	observeBool(&short)
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestUnusedMutatingClosurePreservesAlias(t *testing.T) {
	short := testing.Short()
	unused := func() { short = false }
	_ = unused
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestMutationOfOtherAliasPreservesShort(t *testing.T) {
	short := testing.Short()
	other := false
	mutateOther := func() { other = true }
	mutateOther()
	_ = other
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestUnreachableMutationPreservesShortAlias(t *testing.T) {
	short := testing.Short()
	if false {
		mutateBool(&short)
	}
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestMutationAfterShortReturnDoesNotInvalidateGuard(t *testing.T) {
	short := testing.Short()
	if short {
		return
	}
	mutateBool(&short)
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func TestReturningMutationBranchDoesNotContaminateFallthrough(t *testing.T) {
	short := testing.Short()
	if unknown() {
		mutateBool(&short)
		return
	}
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second {
		t.Fatal("slow")
	}
}

func BenchmarkExcluded(b *testing.B) {
	speedup := measuredSpeedup()
	if speedup < 1.2 {
		b.Fatal("benchmark")
	}
}
