package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5038(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5038.Analyzer, "ps5038")
	// cgo lives in its own package: import "C" turns the WHOLE package
	// into a cgo package. An orphaning rewrite stays advisory there (no
	// fix, golden identical) because a cgo file's import block is never
	// edited.
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5038.Analyzer, "ps5038cgo")
}
