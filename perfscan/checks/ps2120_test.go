package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2120(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2120.Analyzer, "ps2120")
}
