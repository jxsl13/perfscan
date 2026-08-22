package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2115 is advisory (no SuggestedFixes — see the Doc for why no
// bit-identical rewrite exists), so plain Run validates the diagnostics.
func TestPS2115(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2115.Analyzer, "ps2115")
}
