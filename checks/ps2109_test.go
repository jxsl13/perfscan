package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2109(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2109.Analyzer, "ps2109")
}
