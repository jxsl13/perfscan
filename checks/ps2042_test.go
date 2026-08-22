package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2042(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2042.Analyzer, "ps2042")
}
