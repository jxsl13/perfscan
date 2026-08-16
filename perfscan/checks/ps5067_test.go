package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5067(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5067.Analyzer, "ps5067")
}
