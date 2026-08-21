package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5084(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5084.Analyzer, "ps5084", "ps5084alias", "ps5084multi", "ps5084dot")
}
