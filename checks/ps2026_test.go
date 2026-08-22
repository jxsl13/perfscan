package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2026 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the value-receiver forms (var,
// field, slice element, deref, address), the provably-non-nil pointer
// forms (&x, new, both bytes constructors, an aliased import),
// parenthesized shapes, and the advisory guards: an unproven
// *bytes.Buffer receiver (the nil divergence) and comment-bearing
// scaffolding stay unrewritten. See equiv_PS2026_test.go for the runtime
// proof that len(buf.String()) == buf.Len() on every buffer state, and
// that the nil-pointer divergence justifying the gate is real.
func TestPS2026(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2026.Analyzer, "ps2026")
}

// Negative shapes — strings.Builder (whose String is zero-copy),
// same-named methods, interface calls, a shadowed len, promoted methods
// through value or pointer embedding, method values, the already-direct
// spelling, a used (not thrown-away) String result, and a type
// parameter — must produce no diagnostics at all.
func TestPS2026Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2026.Analyzer, "ps2026neg")
}
