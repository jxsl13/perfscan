package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5094(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5094.Analyzer, "ps5094", "ps5094alias", "ps5094comment", "ps5094dot", "ps5094shadow")
}
