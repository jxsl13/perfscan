package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2129(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2129.Analyzer, "ps2129")
}
