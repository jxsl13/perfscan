package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2112 is ADVISORY (no auto-fix): slices.Concat clamps cap to len while
// the chained append over-allocates — an observable divergence pinned by
// TestEquiv_Concat — so there is no golden fixture to apply.
func TestPS2112(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2112.Analyzer, "ps2112")
}
