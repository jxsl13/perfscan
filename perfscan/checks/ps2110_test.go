package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2110(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2110.Analyzer, "ps2110")
}
