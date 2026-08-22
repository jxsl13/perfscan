package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2128(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2128.Analyzer, "ps2128")
}
