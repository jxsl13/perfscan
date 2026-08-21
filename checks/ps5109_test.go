package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5109(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5109.Analyzer,
		"ps5109", "ps5109alias", "ps5109comment", "ps5109dot")
}
