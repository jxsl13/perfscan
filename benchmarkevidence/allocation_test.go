package benchmarkevidence

import (
	"math"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"testing"
)

func TestExactIdentities(t *testing.T) {
	t.Parallel()
	a, err := FromResult(testing.BenchmarkResult{N: 1024, MemBytes: 1025, MemAllocs: 1023, Extra: map[string]float64{"B/op": 99}})
	if err != nil || a.BytesPerOp != 1 || a.BytesRemainder != 1 || a.AllocsPerOp != 0 || a.AllocsRemainder != 1023 {
		t.Fatalf("%+v %v", a, err)
	}
	b, _ := FromResult(testing.BenchmarkResult{N: 1024, MemBytes: 2047, MemAllocs: 1024})
	d, err := PairedDelta(a, b)
	if err != nil || d.MemBytes != 1022 || d.MemAllocs != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	b.BytesRemainder++
	if _, err := PairedDelta(a, b); err == nil {
		t.Fatal("accepted corrupt remainder")
	}
	for _, r := range []testing.BenchmarkResult{{}, {N: -1}} {
		if _, err := FromResult(r); err == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
	// Go versions expose either signed or unsigned totals. Reject negative
	// signed counters and unsigned counters exceeding the supported exact range.
	invalid := testing.BenchmarkResult{N: 1}
	field := reflect.ValueOf(&invalid).Elem().FieldByName("MemBytes")
	if field.Kind() == reflect.Uint64 {
		field.SetUint(math.MaxUint64)
	} else {
		field.SetInt(-1)
	}
	if _, err := FromResult(invalid); err == nil {
		t.Fatal("accepted out-of-range aggregate")
	}
	max, _ := FromResult(testing.BenchmarkResult{N: 1, MemBytes: math.MaxInt64})
	zero, _ := FromResult(testing.BenchmarkResult{N: 1})
	if d, err := PairedDelta(max, zero); err != nil || d.MemBytes != -math.MaxInt64 {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := PairedDelta(a, max); err == nil {
		t.Fatal("accepted different N")
	}
}

func TestRunSubprocess(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"parallel", "fatal", "goexit", "subfatal", "subsuccess", "subgoexit", "cleanupfatal", "cleanuperror", "cleanupgoexit", "skip", "n1", "duration", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			benchtime := "16x"
			if mode == "duration" {
				benchtime = "1ms"
			}
			if mode == "n1" {
				benchtime = "1x"
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestRunChild$", "-test.benchtime="+benchtime)
			cmd.Env = append(os.Environ(), "PERFSCAN_ALLOCATION_CHILD="+mode)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", mode, err, output)
			}
		})
	}
}

func TestRunChild(t *testing.T) {
	mode := os.Getenv("PERFSCAN_ALLOCATION_CHILD")
	if mode == "" {
		t.Skip("subprocess only")
	}
	n := 16
	if mode == "mismatch" {
		n = 17
	}
	if mode == "n1" {
		n = 1
	}
	a, err := Run(n, func(b *testing.B) {
		switch mode {
		case "fatal":
			b.Fatal("nested failure")
		case "goexit":
			runtime.Goexit()
		case "subfatal":
			b.Run("failed", func(b *testing.B) { b.Fatal("subbenchmark failure") })
		case "subsuccess":
			b.Run("success", func(b *testing.B) {})
		case "subgoexit":
			b.Run("exit", func(b *testing.B) { runtime.Goexit() })
		case "cleanupfatal":
			b.Cleanup(func() { b.Fatal("cleanup failure") })
		case "cleanuperror":
			b.Cleanup(func() { b.Error("cleanup failure") })
		case "cleanupgoexit":
			b.Cleanup(func() { runtime.Goexit() })
		case "skip":
			b.Skip("no measurement")
		default:
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					p := make([]byte, 17)
					runtime.KeepAlive(p)
				}
			})
		}
	})
	if mode == "parallel" {
		if err != nil || a.N != 16 {
			t.Fatalf("%+v %v", a, err)
		}
	} else if err == nil {
		t.Fatalf("accepted %s: %+v", mode, a)
	}
}
