package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5013 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both functions (Index and
// LastIndex) in both needle shapes (a one-element []byte{X} composite
// literal whose element carries over verbatim — named constants, integer
// spellings, variables, and side-effecting calls included — and a
// []byte("z") conversion whose literal is wrapped as "z"[0] verbatim over
// escape sequences, raw literals, and non-UTF-8 bytes), parenthesized and
// nested shapes, the aliased-import qualifier, and the advisory guards
// (named-constant conversions and comment-bearing scaffolding stay
// unrewritten). See equiv_PS5013_test.go for the runtime proof that the
// rewrite returns the identical index on every input.
func TestPS5013(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5013.Analyzer, "ps5013")
}

// Negative shapes — multi-byte, empty, and nil needles, non-constant
// conversions, keyed literals, the Any/Rune/Byte members, the strings
// twin, a shadowing identifier, a func value, and a dot import — must
// produce no diagnostics at all.
func TestPS5013Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5013.Analyzer, "ps5013neg")
}
