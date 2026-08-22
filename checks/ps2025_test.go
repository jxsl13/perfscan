package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2025 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both directions (Valid([]byte(s))
// -> ValidString(s) and ValidString(string(b)) -> Valid(b)), the []uint8
// spelling, operands that carry over verbatim (selectors, index
// expressions, side-effecting calls, non-UTF-8 untyped string constants),
// parenthesized shapes, the aliased-import qualifier, and the advisory
// guard (comment-bearing conversion syntax stays unrewritten). See
// equiv_PS2025_test.go for the runtime proof that both directions return
// the identical bool on every input.
func TestPS2025(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2025.Analyzer, "ps2025")
}

// Negative shapes — named string/byte-slice operands, defined conversion
// targets, []byte(nil), the already-direct spellings, no-op same-kind
// conversions, a type parameter, a shadowing identifier with same-named
// methods, other utf8 members, and a dot import — must produce no
// diagnostics at all.
func TestPS2025Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2025.Analyzer, "ps2025neg")
}
