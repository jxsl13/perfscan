package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2052(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2052.Analyzer, "ps2052")
}
