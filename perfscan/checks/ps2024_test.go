package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2024(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2024.Analyzer, "ps2024")
}

// TestPS2024Dot pins the dot-import path: the fix applies with the bare
// sibling name, and a local declaration shadowing that name at the call
// site downgrades the report to advisory (the golden is identical to
// the fixture there).
func TestPS2024Dot(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2024.Analyzer, "ps2024dot")
}
