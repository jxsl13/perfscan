package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5016 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the single needle shape (a
// one-byte constant string literal whose ORIGINAL token is wrapped as
// "z"[0] verbatim over escape sequences, raw literals, and non-UTF-8
// bytes), every operator-introduction context (plain boolean positions
// bare, a leading ! absorbed into < 0, comparison operands and a stacked
// ! parenthesized), the aliased-import qualifier, and the advisory guards
// (named-constant needles, a comment between ! and the call, and go/defer
// call positions stay unrewritten). See equiv_PS5016_test.go for the
// runtime proof that the rewrite returns the identical boolean on every
// input.
func TestPS5016(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5016.Analyzer, "ps5016")
}

// Negative shapes — multi-byte and empty needles, non-constant needles,
// the Any/Rune/Func members, PS5007's Index territory, the bytes twin, a
// shadowing identifier, a func value, and a dot import — must produce no
// diagnostics at all.
func TestPS5016Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5016.Analyzer, "ps5016neg")
}
