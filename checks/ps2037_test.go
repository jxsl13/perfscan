package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2037(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2037.Analyzer, "ps2037")
}
