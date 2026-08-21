package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5126(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5126.Analyzer,
		"ps5126", "ps5126alias", "ps5126comment", "ps5126dot")
}
