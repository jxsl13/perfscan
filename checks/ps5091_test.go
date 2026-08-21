package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5091(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5091.Analyzer, "ps5091", "ps5091alias")
}
