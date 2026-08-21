package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5082(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5082.Analyzer, "ps5082", "ps5082alias", "ps5082dot")
}
