package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5090(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5090.Analyzer, "ps5090", "ps5090alias")
}
