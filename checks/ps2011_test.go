package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2011 is advisory by design (no SuggestedFixes): the rewrite changes
// the result's capacity in escaping contexts and the panic message, so
// only the diagnostics are verified (see equiv_PS2011_test.go for the
// runtime divergence proof that forced the advisory stance).
func TestPS2011(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2011.Analyzer, "ps2011")
}
