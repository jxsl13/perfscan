package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5116(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5116.Analyzer,
		"ps5116", "ps5116alias", "ps5116comment", "ps5116dot", "ps5116orphan")
}
