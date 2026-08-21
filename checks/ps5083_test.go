package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5083(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5083.Analyzer, "ps5083", "ps5083alias", "ps5083multi", "ps5083dot")
}
