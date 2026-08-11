package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2119(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2119.Analyzer, "ps2119")
}
