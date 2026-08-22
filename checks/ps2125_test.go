package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2125(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2125.Analyzer, "ps2125")
}
