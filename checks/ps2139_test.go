package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2139(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2139.Analyzer, "ps2139")
}
