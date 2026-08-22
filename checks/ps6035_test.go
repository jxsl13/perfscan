package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6035(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6035.Analyzer, "ps6035")
}
