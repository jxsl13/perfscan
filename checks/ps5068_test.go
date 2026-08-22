package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5068(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5068.Analyzer, "ps5068")
}
