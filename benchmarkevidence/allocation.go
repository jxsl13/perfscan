// Package benchmarkevidence retains exact process-wide benchmark allocation
// counters for diagnostic experiments. It does not identify allocation sites.
package benchmarkevidence

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"testing"
)

// Allocation contains integer totals and the quotient/remainder identities
// discarded by the standard printed B/op and allocs/op columns.
type Allocation struct {
	N               int   `json:"n"`
	MemBytes        int64 `json:"memBytes"`
	MemAllocs       int64 `json:"memAllocs"`
	BytesPerOp      int64 `json:"bytesPerOp"`
	BytesRemainder  int64 `json:"bytesRemainder"`
	AllocsPerOp     int64 `json:"allocsPerOp"`
	AllocsRemainder int64 `json:"allocsRemainder"`
}

// FromResult preserves exact totals, rejecting absent, invalid, or totals above
// MaxInt64 (so signed paired deltas remain representable without overflow).
// It deliberately ignores Extra metrics, which can override printed columns.
func FromResult(r testing.BenchmarkResult) (Allocation, error) {
	if r.N <= 0 || uint64(r.MemBytes) > math.MaxInt64 || uint64(r.MemAllocs) > math.MaxInt64 {
		return Allocation{}, fmt.Errorf("invalid allocation result: N=%d MemBytes=%d MemAllocs=%d", r.N, r.MemBytes, r.MemAllocs)
	}
	n := int64(r.N)
	bytes, allocs := int64(r.MemBytes), int64(r.MemAllocs)
	return Allocation{r.N, bytes, allocs, bytes / n, bytes % n, allocs / n, allocs % n}, nil
}

// Run invokes an existing benchmark with the predeclared fixed iteration count.
// Call from a diagnostic test binary started with -test.benchtime=<n>x. Do not
// change global testing flags concurrently. Serialization belongs after Run.
// A callback that calls Fatal, FailNow or runtime.Goexit is rejected, including
// failed subbenchmarks: testing.Benchmark does not expose its private log.
// Run requires a leaf benchmark and N >= 2: B.Run aggregates already-rounded
// subresults with N=1, which cannot be relabeled as exact raw totals.
// Do not call raw runtime.Goexit from cleanup: the public API cannot observe a
// cleanup-only exit during the initial probe if a later measured run succeeds.
func Run(n int, benchmark func(*testing.B)) (Allocation, error) {
	benchtime := flag.Lookup("test.benchtime")
	if n < 2 || benchmark == nil || benchtime == nil || benchtime.Value.String() != fmt.Sprintf("%dx", n) {
		return Allocation{}, fmt.Errorf("diagnostic requires fixed N >= 2 and -test.benchtime=%dx", n)
	}
	failed, exited := false, false
	var measured *testing.B
	result := testing.Benchmark(func(b *testing.B) {
		measured = b
		returned := false
		defer func() {
			failed = failed || b.Failed()
			exited = exited || !returned
		}()
		benchmark(b)
		returned = true
	})
	// runN executes cleanup and race checks after our callback's defer.
	if failed || exited || measured == nil || measured.Failed() || measured.Skipped() {
		return Allocation{}, errors.New("diagnostic benchmark failed or exited before returning; retain invocation output and reject its evidence")
	}
	if result.N != n {
		return Allocation{}, fmt.Errorf("diagnostic iteration count mismatch: got %d, want %d", result.N, n)
	}
	return FromResult(result)
}

// Delta is candidate minus baseline for one matched pair. It is not a
// difference of arm medians, a noise correction, or evidence of equivalence.
type Delta struct {
	N         int   `json:"n"`
	MemBytes  int64 `json:"memBytes"`
	MemAllocs int64 `json:"memAllocs"`
}

// PairedDelta validates both integer identities and equal N before subtraction.
func PairedDelta(baseline, candidate Allocation) (Delta, error) {
	for _, sample := range []Allocation{baseline, candidate} {
		if sample.N <= 0 || sample.MemBytes < 0 || sample.MemAllocs < 0 {
			return Delta{}, errors.New("invalid exact allocation totals")
		}
		n := int64(sample.N)
		if sample.BytesPerOp != sample.MemBytes/n || sample.BytesRemainder != sample.MemBytes%n || sample.AllocsPerOp != sample.MemAllocs/n || sample.AllocsRemainder != sample.MemAllocs%n {
			return Delta{}, errors.New("invalid exact allocation identities")
		}
	}
	if baseline.N != candidate.N {
		return Delta{}, fmt.Errorf("paired iteration counts differ: %d and %d", baseline.N, candidate.N)
	}
	// Nonnegative int64 totals make their difference representable in int64.
	return Delta{baseline.N, candidate.MemBytes - baseline.MemBytes, candidate.MemAllocs - baseline.MemAllocs}, nil
}
