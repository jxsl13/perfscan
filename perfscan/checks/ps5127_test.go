package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5127(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5127.Analyzer,
		"ps5127", "ps5127alias", "ps5127comment", "ps5127dot")
}
