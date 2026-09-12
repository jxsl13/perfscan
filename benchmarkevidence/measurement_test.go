package benchmarkevidence

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func TestMeasureRetainsFixedAndAdaptiveNSubprocess(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"fixed", "adaptive", "aggregate", "fatal", "goexit", "cleanupfatal", "cleanuperror", "skip"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			duration := "1ms"
			if mode == "fixed" {
				duration = "16x"
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestMeasureChild$", "-test.benchtime="+duration)
			cmd.Env = append(os.Environ(), "PERFSCAN_MEASUREMENT_CHILD="+mode)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", mode, err, out)
			}
		})
	}
}

func TestMeasureChild(t *testing.T) {
	mode := os.Getenv("PERFSCAN_MEASUREMENT_CHILD")
	if mode == "" {
		t.Skip("subprocess only")
	}
	n := 0
	if mode == "fixed" {
		n = 16
	}
	observation, err := Measure(n, func(b *testing.B) {
		switch mode {
		case "aggregate":
			b.Run("child", func(b *testing.B) {
				for range b.N {
					runtime.KeepAlive(make([]byte, 32))
				}
			})
		case "fatal":
			b.Fatal("benchmark failure")
		case "goexit":
			runtime.Goexit()
		case "cleanupfatal":
			b.Cleanup(func() { b.Fatal("cleanup failure") })
		case "cleanuperror":
			b.Cleanup(func() { b.Error("cleanup failure") })
		case "skip":
			b.Skip("not measured")
		default:
			for range b.N {
				runtime.KeepAlive(make([]byte, 32))
			}
		}
	})
	if mode == "fixed" || mode == "adaptive" {
		if err != nil || observation.Allocation.N < 2 || observation.ElapsedNanos <= 0 {
			t.Fatalf("observation=%+v error=%v", observation, err)
		}
		if n != 0 && observation.Allocation.N != n {
			t.Fatal("lost fixed work")
		}
		if _, err := PairedDelta(observation.Allocation, observation.Allocation); err != nil {
			t.Fatal(err)
		}
	} else if err == nil {
		t.Fatalf("accepted %s: %+v", mode, observation)
	}
}

func TestStoppedTimerAllocationCompatibilitySubprocess(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"run", "measure"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(os.Args[0], "-test.run=^TestStoppedTimerAllocationChild$", "-test.benchtime=16x")
			cmd.Env = append(os.Environ(), "PERFSCAN_STOPPED_TIMER_CHILD="+mode)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", mode, err, out)
			}
		})
	}
}

func TestStoppedTimerAllocationChild(t *testing.T) {
	t.Parallel()
	mode := os.Getenv("PERFSCAN_STOPPED_TIMER_CHILD")
	if mode == "" {
		t.Skip("subprocess only")
	}
	benchmark := func(b *testing.B) {
		b.StopTimer()
		b.ResetTimer()
		for range b.N {
			runtime.KeepAlive(b.N)
		}
	}
	result := testing.Benchmark(benchmark)
	legacy, err := FromResult(result)
	if result.T != 0 || err != nil || legacy != (Allocation{N: 16}) {
		t.Fatalf("allocation-only legacy control: elapsed=%v allocation=%+v error=%v", result.T, legacy, err)
	}
	if mode == "run" {
		allocation, err := Run(16, benchmark)
		if err != nil || allocation != (Allocation{N: 16}) {
			t.Fatalf("allocation-only stopped timer lost exact zero totals: allocation=%+v error=%v", allocation, err)
		}
		return
	}
	if mode != "measure" {
		t.Fatal("unknown subprocess mode")
	}
	if observation, err := Measure(16, benchmark); err == nil {
		t.Fatalf("latency evidence accepted zero elapsed time: %+v", observation)
	}
}
