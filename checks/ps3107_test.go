package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3107(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3107.Analyzer, "ps3107")
}

func TestPS3107Cgo(t *testing.T) {
	// cgo lives in its own package: import "C" turns the WHOLE package
	// into a cgo package. The orphaning rewrite stays advisory there (no
	// fix, golden identical) because a cgo file's import block is never
	// pruned (same policy as PS2118/PS3104).
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3107.Analyzer, "ps3107cgo")
}
