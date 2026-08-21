package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2021(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2021.Analyzer, "ps2021")
}
