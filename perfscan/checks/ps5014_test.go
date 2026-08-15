package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5014 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both needle shapes (a one-element
// []byte{X} composite literal whose element carries over verbatim — named
// constants, integer spellings, variables, and side-effecting calls
// included — and a []byte("z") conversion whose literal is wrapped as
// "z"[0] verbatim over escape sequences, raw literals, and non-UTF-8
// bytes), every operator-introduction context (plain boolean positions
// bare, a leading ! absorbed into < 0, comparison operands and a stacked !
// parenthesized), the aliased-import qualifier, and the advisory guards
// (named-constant conversions, comment-bearing syntax, and go/defer call
// positions stay unrewritten). See equiv_PS5014_test.go for the runtime
// proof that the rewrite returns the identical boolean on every input.
func TestPS5014(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5014.Analyzer, "ps5014")
}

// Negative shapes — multi-byte, empty, and nil needles, non-constant
// conversions, keyed literals, the Any/Rune/Func members, the strings
// twin, PS5013's Index territory, a shadowing identifier, a func value,
// and a dot import — must produce no diagnostics at all.
func TestPS5014Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5014.Analyzer, "ps5014neg")
}
