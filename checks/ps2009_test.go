package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2009(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2009.Analyzer, "ps2009")
}
