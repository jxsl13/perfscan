package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6034(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6034.Analyzer, "ps6034")
}
