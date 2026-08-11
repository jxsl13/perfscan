package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2118(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2118.Analyzer, "ps2118")
}
