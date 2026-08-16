package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2046 is ADVISORY BY DESIGN (AutoFix:false) — the hex.AppendEncode
// rewrite diverges when bs overlaps buf's spare capacity (pinned in
// equiv_PS2046_test.go) — so the fixture is diagnostics-only.
func TestPS2046(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS2046.Analyzer, "ps2046")
}
