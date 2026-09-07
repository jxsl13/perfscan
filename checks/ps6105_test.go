package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6105(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6105.Analyzer, "ps6105")
}
