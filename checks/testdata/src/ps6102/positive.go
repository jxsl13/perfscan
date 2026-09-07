package ps6102

import (
	"testing"
	"time"
)

func work()                    {}
func unknown() bool            { return false }
func measuredSpeedup() float64 { return 1 }

func TestDurationUnguarded(t *testing.T) {
	start := time.Now()
	work()
	elapsed := time.Since(start)
	if elapsed > 2*time.Second { // want `performance threshold "elapsed > 2\*time.Second" in TestDurationUnguarded remains reachable`
		t.Fatal("slow")
	}
}

func TestThroughputUnguarded(t *testing.T) {
	start := time.Now()
	work()
	throughput := 1000 / time.Since(start).Seconds()
	if throughput < 100 { // want `performance threshold "throughput < 100" in TestThroughputUnguarded remains reachable`
		t.Errorf("slow: %f", throughput)
	}
}

func TestAllocationsUnguarded(t *testing.T) {
	allocs := testing.AllocsPerRun(10, work)
	if allocs != 0 { // want `performance threshold "allocs != 0" in TestAllocationsUnguarded remains reachable`
		t.Fail()
	}
}

func TestBenchmarkAllocationsUnguarded(t *testing.T) {
	result := testing.Benchmark(func(b *testing.B) { work() })
	if result.AllocsPerOp() > 1 { // want `performance threshold "result.AllocsPerOp\(\) > 1" in TestBenchmarkAllocationsUnguarded remains reachable`
		panic("allocations")
	}
}

func TestRelativeSpeedupUnguarded(t *testing.T) {
	speedup := measuredSpeedup()
	if speedup < 1.2 { // want `performance threshold "speedup < 1.2" in TestRelativeSpeedupUnguarded remains reachable`
		t.Error("regression")
	}
}

func TestGuardTooLate(t *testing.T) {
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestGuardTooLate remains reachable`
		t.Fatal("slow")
	}
	if testing.Short() {
		t.Skip("too late")
	}
}

func TestWrongPolarity(t *testing.T) {
	if !testing.Short() {
		t.Skip("wrong side")
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestWrongPolarity remains reachable`
		t.Fatal("slow")
	}
}

func TestLogOnlyShortCheck(t *testing.T) {
	if testing.Short() {
		t.Log("short mode")
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestLogOnlyShortCheck remains reachable`
		t.Fatal("slow")
	}
}

func TestUnknownCombinedGuard(t *testing.T) {
	if testing.Short() && unknown() {
		t.Skip("not proved for every short-mode path")
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestUnknownCombinedGuard remains reachable`
		t.Fatal("slow")
	}
}

func TestOverwrittenAlias(t *testing.T) {
	short := testing.Short()
	short = unknown()
	if short {
		t.Skip("mutable alias is not proof")
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestOverwrittenAlias remains reachable`
		t.Fatal("slow")
	}
}

func TestThresholdInShortBranch(t *testing.T) {
	latency := time.Since(time.Now())
	if testing.Short() && latency > time.Second { // want `performance threshold "latency > time.Second" in TestThresholdInShortBranch remains reachable`
		t.Fatal("slow")
	}
}

func TestConstantFalseDoesNotGuard(t *testing.T) {
	if testing.Short() && false {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestConstantFalseDoesNotGuard remains reachable`
		t.Fatal("slow")
	}
}

func TestWrongBooleanComparison(t *testing.T) {
	short := testing.Short()
	if short == false {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestWrongBooleanComparison remains reachable`
		t.Fatal("slow")
	}
}

func TestRealSubtests(t *testing.T) {
	t.Run("campaign", func(t *testing.T) {
		speedup := measuredSpeedup()
		if speedup < 1.2 { // want `performance threshold "speedup < 1.2" in TestRealSubtests/campaign remains reachable`
			t.Fatal("slow")
		}
	})
	t.Run("outer", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) {
			allocations := testing.AllocsPerRun(1, work)
			if allocations > 0 { // want `performance threshold "allocations > 0" in TestRealSubtests/outer/inner remains reachable`
				t.Fatal("allocating")
			}
		})
	})
	ok := t.Run("assigned", func(t *testing.T) {
		bandwidth := 10.0
		if bandwidth < 20 { // want `performance threshold "bandwidth < 20" in TestRealSubtests/assigned remains reachable`
			t.Fatal("slow")
		}
	})
	_ = ok
}

func assertSpeedup(t *testing.T, speedup float64) {
	if speedup < 1.2 { // want `performance threshold "speedup < 1.2" in TestCalledHelper remains reachable`
		t.Fatal("slow")
	}
}

func TestCalledHelper(t *testing.T) {
	assertSpeedup(t, measuredSpeedup())
}

func TestCalledClosure(t *testing.T) {
	assert := func(allocationCount float64) {
		if allocationCount > 0 { // want `performance threshold "allocationCount > 0" in TestCalledClosure remains reachable`
			t.Fatal("allocating")
		}
	}
	assert(testing.AllocsPerRun(1, work))
}

func mutateBool(pointer *bool) { *pointer = false }

func TestPointerHelperMutatesShortAlias(t *testing.T) {
	short := testing.Short()
	mutateBool(&short)
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestPointerHelperMutatesShortAlias remains reachable`
		t.Fatal("slow")
	}
}

func TestCapturedMutationInvalidatesShortAlias(t *testing.T) {
	short := testing.Short()
	mutate := func() { short = false }
	mutate()
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestCapturedMutationInvalidatesShortAlias remains reachable`
		t.Fatal("slow")
	}
}

func forwardBoolMutation(pointer *bool) { mutateBool(pointer) }

func TestForwardedPointerMutationInvalidatesAlias(t *testing.T) {
	short := testing.Short()
	forwardBoolMutation(&short)
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestForwardedPointerMutationInvalidatesAlias remains reachable`
		t.Fatal("slow")
	}
}

func TestLocalPointerAliasMutationInvalidatesShort(t *testing.T) {
	short := testing.Short()
	pointer := &short
	mutateBool(pointer)
	if short {
		return
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestLocalPointerAliasMutationInvalidatesShort remains reachable`
		t.Fatal("slow")
	}
}

func TestKnownSwitchSelectsReachableDefault(t *testing.T) {
	switch testing.Short() {
	case false:
		return
	default:
		latency := time.Since(time.Now())
		if latency > time.Second { // want `performance threshold "latency > time.Second" in TestKnownSwitchSelectsReachableDefault remains reachable`
			t.Fatal("slow")
		}
	}
}

func TestUnknownLoopMayFinish(t *testing.T) {
	for unknown() {
		t.Skip("only if the loop executes")
	}
	latency := time.Since(time.Now())
	if latency > time.Second { // want `performance threshold "latency > time.Second" in TestUnknownLoopMayFinish remains reachable`
		t.Fatal("slow")
	}
}

func TestNonemptyRangeExecutes(t *testing.T) {
	for range [1]int{} {
		latency := time.Since(time.Now())
		if latency > time.Second { // want `performance threshold "latency > time.Second" in TestNonemptyRangeExecutes remains reachable`
			t.Fatal("slow")
		}
	}
}
