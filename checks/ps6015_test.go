package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6015(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6015.Analyzer, "ps6015")
}
