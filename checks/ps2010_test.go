package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2010(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2010.Analyzer, "ps2010")
}

// TestPS2010Cgo pins the cgo import-orphan guard: in a cgo file whose
// only bytes reference is the fixable Equal call, the fix is withheld
// (the import block of a cgo file is never pruned) and the report stays
// advisory — the golden is identical to the fixture.
func TestPS2010Cgo(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2010.Analyzer, "ps2010cgo")
}
