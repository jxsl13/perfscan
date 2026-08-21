package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5120(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5120.Analyzer,
		"ps5120", "ps5120alias", "ps5120comment", "ps5120dot")
}
