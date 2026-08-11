package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2114(t *testing.T) {
	// Advisory check: no SuggestedFixes, analysistest.Run validates the
	// // want diagnostics only.
	analysistest.Run(t, analysistest.TestData(), PS2114.Analyzer, "ps2114")
}
