package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6033(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6033.Analyzer, "ps6033")
}
