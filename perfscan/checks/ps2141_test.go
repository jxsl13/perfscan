package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2141(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2141.Analyzer, "ps2141")
}
