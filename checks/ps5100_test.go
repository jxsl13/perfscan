package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5100(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5100.Analyzer,
		"ps5100", "ps5100alias", "ps5100comment", "ps5100dot")
}
