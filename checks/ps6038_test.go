package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6038(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6038.Analyzer, "ps6038")
}
