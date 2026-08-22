package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2015(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2015.Analyzer, "ps2015")
}
