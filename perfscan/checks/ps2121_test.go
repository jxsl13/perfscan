package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2121(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2121.Analyzer, "ps2121")
}
