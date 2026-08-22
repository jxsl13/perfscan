package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6009(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6009.Analyzer, "ps6009", "ps6009alias")
}
