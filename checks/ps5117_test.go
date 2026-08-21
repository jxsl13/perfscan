package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5117(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5117.Analyzer,
		"ps5117", "ps5117alias", "ps5117comment", "ps5117dot")
}
