package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2050 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. Positives cover a rune and an int32 operand and a
// NAMED string conversion target (kept as T(r)). Advisory: a comment in
// the deleted scaffolding. Negatives stay SILENT: a non-nil buffer (would
// prepend its bytes), a constant rune (string(<const>) is a different, vet-
// flagged conversion), and a []byte target. See equiv_PS2050_test.go.
func TestPS2050(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2050.Analyzer, "ps2050")
}
