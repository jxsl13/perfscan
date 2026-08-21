package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5098(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5098.Analyzer,
		"ps5098", "ps5098alias", "ps5098comment", "ps5098dot")
}
