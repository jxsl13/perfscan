package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2014(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2014.Analyzer, "ps2014")
}
