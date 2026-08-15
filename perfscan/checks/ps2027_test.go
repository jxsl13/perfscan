package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2027 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers every rewritten comparison form
// (String() against "" with == / != in both operand orders,
// len(String()) against an integer constant with every comparison
// operator), receivers that carry over verbatim (struct fields, slice
// and array elements, a named empty-string constant, a type alias),
// the provably-non-nil pointer shapes (&buf, new(bytes.Buffer)),
// parenthesized variants, and the advisory guards (a not-provably-
// non-nil *bytes.Buffer, comment-bearing syntax). See
// equiv_PS2027_test.go for the runtime proof that Before and After
// agree on every buffer state — and for the nil-receiver divergence
// that keeps pointer receivers advisory.
func TestPS2027(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2027.Analyzer, "ps2027")
}

// Negative shapes — strings.Builder, fmt.Stringer, user-defined String
// methods, Buffer embeds (value and pointer), comparisons that are not
// constant emptiness tests, a bare len(buf.String()) value, and a
// shadowed len — must produce no diagnostics at all.
func TestPS2027Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2027.Analyzer, "ps2027neg")
}
