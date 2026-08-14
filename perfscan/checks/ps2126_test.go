package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2126(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2126.Analyzer, "ps2126")
}
