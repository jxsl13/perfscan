package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5085(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5085.Analyzer, "ps5085", "ps5085alias")
}
