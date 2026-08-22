package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2123(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2123.Analyzer, "ps2123")
	// cgo lives in its own package: import "C" turns the WHOLE package
	// into a cgo package. An orphaning rewrite stays advisory there (no
	// fix, golden identical) because a cgo file's import block is never
	// pruned.
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2123.Analyzer, "ps2123cgo")
}
