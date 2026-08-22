package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2031 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both operators in both operand
// orders, constant operands that carry over verbatim (named constants,
// raw strings, invalid-UTF-8 and NUL escapes, constant concatenation),
// receivers that carry over verbatim (struct fields, slice and array
// elements, a type alias), the provably-non-nil pointer shapes (&buf,
// new(bytes.Buffer)), an aliased import of bytes reused under its
// alias, parenthesized variants, and a != embedded under && (the
// prefixed ! needs no parentheses). The advisory guards live in
// ps2031adv: a not-provably-non-nil *bytes.Buffer, a named-bool result
// context (bytes.Equal's typed bool would not compile there), a file
// with no usable bytes qualifier (missing import, dot import, shadowed
// name), and comment-bearing syntax. See equiv_PS2031_test.go for the
// runtime proof that Before and After agree on every buffer state —
// and for the nil-receiver divergence that keeps pointer receivers
// advisory.
func TestPS2031(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2031.Analyzer, "ps2031", "ps2031adv")
}

// Negative shapes — the empty string (PS2027's pattern), the len form,
// non-constant far operands, ordered comparisons, strings.Builder /
// fmt.Stringer / user String methods, and Buffer embeds — must produce
// no diagnostics at all.
func TestPS2031Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2031.Analyzer, "ps2031neg")
}
