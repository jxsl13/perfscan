package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2138(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2138.Analyzer, "ps2138")
}
