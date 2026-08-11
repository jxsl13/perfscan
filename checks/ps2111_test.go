package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2111(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2111.Analyzer, "ps2111")
}
