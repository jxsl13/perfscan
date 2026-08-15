package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS3029(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3029.Analyzer, "ps3029")
}
