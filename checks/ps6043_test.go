package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6043(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6043.Analyzer, "ps6043")
}
