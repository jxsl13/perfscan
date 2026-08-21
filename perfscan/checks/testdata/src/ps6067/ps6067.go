package ps6067

import (
	"os"
	"runtime"
	"testing"
	"time"
)

var dedicatedPerformanceTests bool

func work()                               {}
func deviceNanoseconds() int64            { return 1 }
func loadStoredBaseline() time.Duration   { return time.Second }
func requireDedicatedRunner(t *testing.T) {}
func runningOnDedicatedRunner() bool      { return false }
func runPerformanceTest()                 {}
func lastGPUSeconds() float64             { return 0 }

func TestSinceCeiling(t *testing.T) {
	start := time.Now()
	work()
	elapsed := time.Since(start)
	if elapsed > 40*time.Microsecond { // want `time.Since-derived elapsed time is compared with an absolute performance threshold`
		t.Fatalf("elapsed %v", elapsed)
	}
}

func TestSubCeiling(t *testing.T) {
	start := time.Now()
	work()
	elapsed := time.Now().Sub(start)
	if elapsed >= time.Millisecond { // want `time.Time.Sub-derived elapsed time is compared with an absolute performance threshold`
		t.Error("too slow")
	}
}

func TestBenchmarkCeiling(t *testing.T) {
	result := testing.Benchmark(func(b *testing.B) {
		for b.Loop() {
			work()
		}
	})
	if result.NsPerOp() > 1_000 { // want `testing.Benchmark NsPerOp is compared with an absolute performance threshold`
		t.Errorf("slow: %d", result.NsPerOp())
	}
}

func TestBenchmarkReportedThroughput(t *testing.T) {
	result := testing.Benchmark(func(b *testing.B) {
		for b.Loop() {
			b.ReportMetric(2_000_000, "items/s")
		}
	})
	if result.Extra["items/s"] < 1_000_000 { // want `testing.Benchmark result is compared with an absolute performance threshold`
		t.Error("throughput floor")
	}
}

func TestThroughputFloor(t *testing.T) {
	start := time.Now()
	work()
	rate := 1_000 / time.Since(start).Seconds()
	if rate < 1_000_000 { // want `time.Since-derived elapsed time is compared with an absolute performance threshold`
		panic("throughput floor")
	}
}

func TestDeviceDurationCeiling(t *testing.T) {
	deviceElapsed := time.Duration(deviceNanoseconds())
	ceiling := 50 * time.Microsecond
	if deviceElapsed > ceiling { // want `device elapsed/latency timing is compared with an absolute performance threshold`
		t.Fatal("device too slow")
	}
}

func TestUnixNanoDelta(t *testing.T) {
	start := time.Now()
	elapsedNanoseconds := time.Now().UnixNano() - start.UnixNano()
	if elapsedNanoseconds > 40_000 { // want `time.Now-derived clock delta is compared with an absolute performance threshold`
		t.Fatal("slow")
	}
}

func TestElseFailure(t *testing.T) {
	start := time.Now()
	elapsed := time.Since(start)
	if elapsed <= 40*time.Microsecond { // want `time.Since-derived elapsed time is compared with an absolute performance threshold`
		t.Log("fast")
	} else {
		t.Fatalf("slow")
	}
}

func BenchmarkExcluded(b *testing.B) {
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond {
		b.Fatal("benchmarks are excluded")
	}
}

func TestMetricOnly(t *testing.T) {
	start := time.Now()
	elapsed := time.Since(start)
	if elapsed > 40*time.Microsecond {
		t.Logf("metric: %v", elapsed)
	}
}

func TestEnvironmentGated(t *testing.T) {
	if os.Getenv("RUN_DEDICATED_PERF") != "1" {
		t.Skip("requires controlled runner")
	}
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond {
		t.Fatal("slow")
	}
}

func TestLookupEnvironmentGated(t *testing.T) {
	_, enabled := os.LookupEnv("RUN_DEDICATED_PERF")
	if !enabled {
		t.SkipNow()
	}
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond {
		t.Fatal("slow")
	}
}

func TestFlagGated(t *testing.T) {
	if !dedicatedPerformanceTests {
		t.Skip("opt in")
	}
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond {
		t.Fatal("slow")
	}
}

func TestSameConditionOptIn(t *testing.T) {
	start := time.Now()
	if os.Getenv("STRICT_PERF") == "1" && time.Since(start) > 40*time.Microsecond {
		t.Fatal("slow")
	}
}

func TestEnclosingEnvironmentOptIn(t *testing.T) {
	if os.Getenv("STRICT_PERF") == "1" {
		start := time.Now()
		if time.Since(start) > 40*time.Microsecond {
			t.Fatal("gated")
		}
	}
}

func TestDedicatedHelperOptIn(t *testing.T) {
	requireDedicatedRunner(t)
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond {
		t.Fatal("gated")
	}
}

func TestEnclosingDedicatedHelper(t *testing.T) {
	if runningOnDedicatedRunner() {
		start := time.Now()
		if time.Since(start) > 40*time.Microsecond {
			t.Fatal("gated")
		}
	}
}

func TestWorkloadNameIsNotGuard(t *testing.T) {
	runPerformanceTest()
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond { // want `time.Since-derived elapsed time is compared with an absolute performance threshold`
		t.Fatal("workload call is not an opt-in")
	}
}

func TestRelativeControl(t *testing.T) {
	startControl := time.Now()
	work()
	controlElapsed := time.Since(startControl)
	startCandidate := time.Now()
	work()
	candidateElapsed := time.Since(startCandidate)
	ratio := float64(candidateElapsed) / float64(controlElapsed)
	if ratio > 1.10 {
		t.Fatalf("relative regression")
	}
}

func TestStoredBaseline(t *testing.T) {
	start := time.Now()
	elapsed := time.Since(start)
	baseline := loadStoredBaseline()
	if elapsed > baseline {
		t.Fatal("stored baseline comparison")
	}
}

func TestStoredBaselineRatio(t *testing.T) {
	start := time.Now()
	elapsed := time.Since(start)
	baseline := loadStoredBaseline()
	ratio := float64(elapsed) / float64(baseline)
	if ratio > 1.10 {
		t.Fatal("statistical/stored baseline comparison")
	}
}

func TestFunctionalDuration(t *testing.T) {
	got := time.Hour
	if got > time.Minute {
		t.Fatal("ordinary duration behavior, not a timing sample")
	}
}

func TestPlatformOnlyIsNotRunnerGate(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Metal only")
	}
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond { // want `time.Since-derived elapsed time is compared with an absolute performance threshold`
		t.Fatal("still unstable on shared macOS")
	}
}

//perfscan:absolute-performance-ceiling-validated dedicated M2 runner with frozen thermal policy.
func TestValidatedCeiling(t *testing.T) {
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond {
		t.Fatal("externally validated")
	}
}

func TestNoFailure(t *testing.T) {
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond {
		return
	}
}

type fakeT struct{}

func (*fakeT) Fatal(args ...any) {}

func TestUserFatal(t *testing.T) {
	fake := &fakeT{}
	start := time.Now()
	if time.Since(start) > 40*time.Microsecond {
		fake.Fatal("not testing.T")
	}
}

// TestInvokedClosureCeiling pins the local slope helper and numeric ceiling
// shape from the owner-reported Metal shared-runner failure.
func TestInvokedClosureCeiling(t *testing.T) {
	slope := func(name string, ceiling float64) {
		measure := func(n int) float64 {
			best := 1e18
			for range 15 {
				if elapsed := lastGPUSeconds(); elapsed < best {
					best = elapsed
				}
			}
			return best
		}
		lo, hi := measure(16), measure(128)
		us := (hi - lo) / (128 - 16) * 1e6
		if us > ceiling { // want `named elapsed/device timing is compared with an absolute performance threshold`
			t.Errorf("%s marginal cost %.2f us exceeds %.2f us", name, us, ceiling)
		}
	}
	slope("RMS norm", 60)
	slope("residual add", 40)
	dynamicCeiling := float64(time.Now().UnixNano())
	slope("attention", dynamicCeiling)
}

func TestDeadClosureIsExcluded(t *testing.T) {
	unused := func(ceiling float64) {
		us := lastGPUSeconds() * 1e6
		if us > ceiling {
			t.Fatal("dead helper")
		}
	}
	_ = unused
}

func TestDynamicClosureThresholdIsExcluded(t *testing.T) {
	check := func(ceiling float64) {
		us := lastGPUSeconds() * 1e6
		if us > ceiling {
			t.Fatal("dynamic baseline")
		}
	}
	check(float64(time.Now().UnixNano()))
}

func TestTransitiveClosureThreshold(t *testing.T) {
	check := func(us, limit float64) {
		if us > limit { // want `named elapsed/device timing is compared with an absolute performance threshold`
			t.Fatal("transitive hard ceiling")
		}
	}
	slope := func(ceiling float64) {
		check(lastGPUSeconds()*1e6, ceiling)
	}
	slope(40)
}

func TestImmediateClosureThreshold(t *testing.T) {
	(func(limit float64) {
		us := lastGPUSeconds() * 1e6
		if us > limit { // want `device elapsed/latency timing is compared with an absolute performance threshold`
			t.Fatal("immediate hard ceiling")
		}
	})(40)
}

func TestReassignedClosureIsExcluded(t *testing.T) {
	check := func(float64) {}
	check(40)
	check = func(limit float64) {
		us := lastGPUSeconds() * 1e6
		if us > limit {
			t.Fatal("this literal was not called")
		}
	}
	_ = check
}
