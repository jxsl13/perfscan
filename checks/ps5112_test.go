package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5112(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5112.Analyzer,
		"ps5112", "ps5112alias", "ps5112comment", "ps5112dot", "ps5112orphan")
}
