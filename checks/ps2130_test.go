package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2130(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2130.Analyzer, "ps2130")
}
