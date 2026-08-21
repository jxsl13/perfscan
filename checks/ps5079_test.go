package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5079(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5079.Analyzer, "ps5079", "ps5079alias", "ps5079orphan")
}
