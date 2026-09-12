//go:build allocationdiagnostic

package benchmarks

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/jxsl13/perfscan/benchmarkevidence"
)

// TestAllocationDiagnostic wraps existing functions without editing their
// measured body. Build the same added file/options into both pinned binaries.
// Select one arm per fresh process; A/B control both select Before.
func TestAllocationDiagnostic(t *testing.T) {
	arm := os.Getenv("PERFSCAN_ALLOCATION_ARM")
	benchmark := BenchmarkPS2002_Before
	if arm == "after" {
		benchmark = BenchmarkPS2002_After
	} else if arm != "before" {
		t.Fatal("set PERFSCAN_ALLOCATION_ARM to before or after")
	}
	result, err := benchmarkevidence.Run(1024, benchmark)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		t.Fatal(err)
	}
}
