package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS3106 is advisory (AutoFix:false, no SuggestedFixes), so analysistest.Run
// validates the // want diagnostics only — there is no .golden file.
func TestPS3106(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS3106.Analyzer, "ps3106")
}
