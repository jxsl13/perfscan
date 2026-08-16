package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2053(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2053.Analyzer, "ps2053")
}
