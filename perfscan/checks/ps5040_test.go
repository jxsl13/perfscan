package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5040 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. The suite covers the plain rune and int32 forms
// (operand verbatim), the narrower widths (byte, int16) wrapped as
// rune(x), a side-effecting operand passing through once, and reuse of
// the file's existing unicode/utf8 qualifier. Advisory cases pin the
// withheld-fix guards: a NAMED byte-slice dst and a comment inside the
// rewritten scaffolding. Negatives stay SILENT: wider integer kinds
// (int, uint32 — a value past int32 makes fmt emit U+FFFD where rune(x)
// truncates), a constant operand, and non-"%c" formats. See
// equiv_PS5040_test.go for the runtime byte-identity proof.
func TestPS5040(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5040.Analyzer, "ps5040")
}

// TestPS5040Add pins the import-add path: the first fix inserts unicode/utf8.
func TestPS5040Add(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5040.Analyzer, "ps5040add")
}
