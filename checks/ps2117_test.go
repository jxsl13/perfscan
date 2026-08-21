package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2117(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2117.Analyzer, "ps2117")
}
