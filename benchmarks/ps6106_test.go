package benchmarks

import (
	"os"
	"os/exec"
	"testing"
)

const (
	ps6106Full   = 1024
	ps6106Active = 16
)

var (
	ps6106WorkScratch [ps6106Full]float32
	ps6106WorkSink    float32
)

// This pair isolates the bounded fused-epilogue remedy. It is deliberately
// distinct from PS6106's documentation example, which uses a separate
// extent-aware activation call.
func ps6106BeforeFusedWork() float32 {
	for index := 0; index < ps6106Active; index++ {
		ps6106WorkScratch[index] = float32(index) + 1
	}
	for index := range ps6106WorkScratch {
		ps6106WorkScratch[index] *= 1.5
	}
	var result float32
	for index := 0; index < ps6106Active; index++ {
		result += ps6106WorkScratch[index]
	}
	return result
}

func ps6106AfterBoundedFusedWork() float32 {
	for index := 0; index < ps6106Active; index++ {
		ps6106WorkScratch[index] = (float32(index) + 1) * 1.5
	}
	var result float32
	for index := 0; index < ps6106Active; index++ {
		result += ps6106WorkScratch[index]
	}
	return result
}

func BenchmarkPS6106_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps6106WorkSink = ps6106BeforeFusedWork()
	}
}

func BenchmarkPS6106_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps6106WorkSink = ps6106AfterBoundedFusedWork()
	}
}

func TestPS6106WorkPair(t *testing.T) {
	if os.Getenv("PERFSCAN_PS6106_WORK_CHILD") != "1" {
		t.Parallel()
		command := exec.Command(os.Args[0], "-test.run=^TestPS6106WorkPair$", "-test.count=1")
		command.Env = append(os.Environ(), "PERFSCAN_PS6106_WORK_CHILD=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("isolated work-pair child failed: %v\n%s", err, output)
		}
		return
	}
	for index := range ps6106WorkScratch {
		ps6106WorkScratch[index] = 0
	}
	before := ps6106BeforeFusedWork()
	ps6106WorkScratch[ps6106Active] = 7
	after := ps6106AfterBoundedFusedWork()
	if before != after {
		t.Fatalf("active result differs: before %v, after %v", before, after)
	}
	if ps6106WorkScratch[ps6106Active] != 7 {
		t.Fatalf("bounded arm changed inactive tail: %v", ps6106WorkScratch[ps6106Active])
	}
	if allocations := testing.AllocsPerRun(100, func() { ps6106WorkSink = ps6106BeforeFusedWork() }); allocations != 0 {
		t.Fatalf("before allocations/run = %v, want 0", allocations)
	}
	if allocations := testing.AllocsPerRun(100, func() { ps6106WorkSink = ps6106AfterBoundedFusedWork() }); allocations != 0 {
		t.Fatalf("after allocations/run = %v, want 0", allocations)
	}
}
