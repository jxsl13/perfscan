package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6057(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6057.Analyzer, "ps6057")
}
