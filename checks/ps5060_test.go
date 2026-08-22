package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5060(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5060.Analyzer, "ps5060")
}
