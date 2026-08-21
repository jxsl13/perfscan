package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6027(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6027.Analyzer, "ps6027")
}
