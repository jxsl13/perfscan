package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6074(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6074.Analyzer, "ps6074")
}
