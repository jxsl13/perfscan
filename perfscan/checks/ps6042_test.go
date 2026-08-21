package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6042(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6042.Analyzer, "ps6042")
}
