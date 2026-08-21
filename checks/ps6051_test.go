package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6051(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6051.Analyzer, "ps6051")
}
