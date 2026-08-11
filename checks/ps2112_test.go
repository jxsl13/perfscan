package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2112(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2112.Analyzer, "ps2112")
}
