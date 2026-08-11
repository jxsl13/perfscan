package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5102(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5102.Analyzer, "ps5102")
}
