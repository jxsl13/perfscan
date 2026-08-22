package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5099(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5099.Analyzer,
		"ps5099", "ps5099alias", "ps5099comment", "ps5099dot")
}
