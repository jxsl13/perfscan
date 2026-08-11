package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2113(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2113.Analyzer, "ps2113")
}
