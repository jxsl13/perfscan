package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3024(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3024.Analyzer, "ps3024")
	// cgo lives in its own package: import "C" turns the WHOLE package
	// into a cgo package, which must stay away from the plain fixtures.
	// The finding is advisory there (no fix, golden identical) because a
	// cgo file's import block must never be edited.
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3024.Analyzer, "ps3024cgo")
}
