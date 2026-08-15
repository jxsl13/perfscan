package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2038 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both directions
// (DecodeRuneInString(string(b)) -> DecodeRune(b) and
// DecodeRune([]byte(s)) -> DecodeRuneInString(s)) for both the first-
// and last-rune pairs, the []uint8 spelling, operands that carry over
// verbatim (selectors, index expressions, side-effecting calls, untyped
// string constants), parenthesized shapes, the aliased-import
// qualifier, and the advisory guard (comment-bearing conversion syntax
// stays unrewritten). See equiv_PS2038_test.go for the runtime proof
// that all four pairs return the identical (r, size) on every input.
func TestPS2038(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2038.Analyzer, "ps2038")
}

// Negative shapes — named string/byte-slice operands, defined
// conversion targets, []byte(nil), the already-direct spellings, no-op
// same-kind conversions, []rune/[]int32/rune sources (string([]rune)
// ENCODES, it does not copy bytes), a stored conversion, a same-named
// local function, a shadowing identifier with same-named methods, other
// utf8 members, and a dot import — must produce no diagnostics at all.
func TestPS2038Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2038.Analyzer, "ps2038neg")
}
