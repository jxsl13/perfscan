package checks_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/checks"
)

func TestPS6101Round3External(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), checks.PS6101.Analyzer, "ps6101round3")
}
