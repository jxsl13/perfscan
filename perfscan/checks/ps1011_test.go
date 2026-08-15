package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS1011(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS1011.Analyzer, "ps1011")
}
