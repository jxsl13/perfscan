package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2039(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2039.Analyzer, "ps2039")
}
