package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2020(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2020.Analyzer, "ps2020")
}
