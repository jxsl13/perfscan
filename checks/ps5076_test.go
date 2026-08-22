package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5076(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5076.Analyzer, "ps5076", "ps5076alias")
}
