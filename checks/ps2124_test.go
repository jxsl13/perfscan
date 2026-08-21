package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2124(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2124.Analyzer, "ps2124")
}
