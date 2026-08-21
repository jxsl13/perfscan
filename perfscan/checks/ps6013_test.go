package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6013(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6013.Analyzer, "ps6013")
}
