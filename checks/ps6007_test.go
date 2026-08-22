package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6007(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6007.Analyzer, "ps6007", "ps6007alias")
}
