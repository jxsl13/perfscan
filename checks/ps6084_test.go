package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6084(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6084.Analyzer, "ps6084")
}
