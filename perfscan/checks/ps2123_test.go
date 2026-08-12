package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2123(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2123.Analyzer, "ps2123")
}
