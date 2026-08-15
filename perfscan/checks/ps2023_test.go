package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2023(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2023.Analyzer, "ps2023")
}
