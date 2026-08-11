package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5103 is advisory (AutoFix:false, no SuggestedFixes), so plain
// analysistest.Run validates the // want diagnostics; there is no golden
// file.
func TestPS5103(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS5103.Analyzer, "ps5103")
}
