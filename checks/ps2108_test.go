package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2108(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2108.Analyzer, "ps2108")
}
