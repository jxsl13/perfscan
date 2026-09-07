package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6102(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6102.Analyzer, "ps6102", "ps6102alias")
}
