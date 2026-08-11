package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2122(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2122.Analyzer, "ps2122")
}
