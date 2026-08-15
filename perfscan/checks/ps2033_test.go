package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2033(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2033.Analyzer, "ps2033")
}
