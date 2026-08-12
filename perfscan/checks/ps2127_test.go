package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2127(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2127.Analyzer, "ps2127")
}
