package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5081(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5081.Analyzer, "ps5081", "ps5081alias", "ps5081multi")
}
