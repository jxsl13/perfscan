package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2116(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2116.Analyzer, "ps2116")
}
