package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5026 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the bytes.ContainsRune ->
// IndexByte >= 0 rewrite with the rune literal carried over verbatim
// (escape sequences, NUL, 0x7f and plain integer spellings included), every
// operator-introduction context (plain boolean positions bare, a leading !
// absorbed into < 0, comparison operands and a stacked ! parenthesized),
// the aliased-import qualifier, and the advisory guards (named-constant
// and constant-expression needles, go/defer and bare-statement positions,
// and a comment between ! and the call stay unrewritten). See
// equiv_PS5026_test.go for the runtime proof that the rewrite returns the
// identical boolean on every input — and for the divergence witnesses
// pinning why the [0, 0x80) needle bound is load-bearing.
func TestPS5026(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5026.Analyzer, "ps5026")
}

// Negative shapes — non-constant runes, constant runes outside [0, 0x80)
// (multi-byte, RuneError, invalid, negative), the strings twin owned by
// PS5024 and bytes.IndexRune owned by PS5023 (out of scope), a shadowing
// identifier, a func value, and a dot import — must produce no
// diagnostics at all.
func TestPS5026Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5026.Analyzer, "ps5026neg")
}
