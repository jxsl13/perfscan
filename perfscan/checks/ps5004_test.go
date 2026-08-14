package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5004(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5004.Analyzer, "ps5004")
}
