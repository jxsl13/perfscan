package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6047(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6047.Analyzer, "ps6047")
}
