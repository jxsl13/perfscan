package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5123(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5123.Analyzer,
		"ps5123", "ps5123alias", "ps5123comment", "ps5123dot")
}
