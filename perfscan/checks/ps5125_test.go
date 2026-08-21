package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5125(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5125.Analyzer,
		"ps5125", "ps5125alias", "ps5125comment", "ps5125dot")
}
