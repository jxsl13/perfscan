package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6039(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6039.Analyzer, "ps6039")
}
