package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2029 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers all eight membership-equivalent
// comparison shapes with the literal on either side of the operator,
// an aliased strings import, the bytes twin, arguments that carry over
// byte-verbatim (struct fields, index expressions, calls, redundant
// parens), redundant parens around the scaffolding, the chain-aware
// IndexByte fixed points (one-byte string literal, one-element
// []byte{X}, []byte("z") conversion), named-constant separators that
// keep the plain Contains spelling, the rewrite inside a larger
// condition, and the advisory guard (comment-bearing scaffolding). See
// equiv_PS2029_test.go for the runtime proof that
// len(SplitN(s, sep, 2)) == 2 and Contains(s, sep) agree on every
// input with a non-empty separator.
func TestPS2029(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2029.Analyzer, "ps2029")
}

// Negative shapes — variable and empty separators (the divergence
// shapes), limits other than the literal 2, comparisons that are not
// membership tests, non-literal comparands, stored SplitN slices,
// Split/SplitAfterN, named-bool contexts, local/method SplitN,
// shadowed strings and shadowed len — must produce no diagnostics at
// all.
func TestPS2029Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2029.Analyzer, "ps2029neg")
}
