package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5007 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both functions (Index and
// LastIndex), the literal-verbatim [0] wrap over escape sequences, raw
// literals and non-UTF-8 bytes, parenthesized and nested shapes, the
// aliased-import qualifier, and the advisory guards (named constants and
// constant expressions stay unrewritten). See equiv_PS5007_test.go for
// the runtime proof that the rewrite returns the identical index on
// every input.
func TestPS5007(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5007.Analyzer, "ps5007")
}

// Negative shapes — multi-byte and empty needles, non-constant needles,
// the Any/Rune/Byte members, the bytes twin, a shadowing identifier, a
// func value, and a dot import — must produce no diagnostics at all.
func TestPS5007Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5007.Analyzer, "ps5007neg")
}
