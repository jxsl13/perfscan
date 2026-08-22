package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6014(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6014.Analyzer, "ps6014", "ps6014alias")
}
