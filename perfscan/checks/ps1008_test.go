package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS1008(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS1008.Analyzer, "ps1008")
}
