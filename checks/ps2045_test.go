package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2045 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers both operators, receivers that
// carry over verbatim (locals, struct fields, slice and array
// elements, a type alias, the same receiver on both sides), the
// provably-non-nil pointer shapes (&buf, new(bytes.Buffer)) on either
// or both sides, an aliased import of bytes reused under its alias,
// parenthesized variants, and a != embedded under && (the prefixed !
// needs no parentheses). The advisory guards live in ps2045adv: a
// not-provably-non-nil *bytes.Buffer on either side, a named-bool
// result context (bytes.Equal's typed bool would not compile there), a
// file with no usable bytes qualifier (missing import, dot import,
// shadowed name), and comment-bearing syntax. See equiv_PS2045_test.go
// for the runtime proof that Before and After agree on every pair of
// buffer states — and for the nil-receiver divergence that keeps
// pointer receivers advisory.
func TestPS2045(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2045.Analyzer, "ps2045", "ps2045adv")
}

// Negative shapes — a constant far operand (PS2031's pattern), a
// non-call operand, ordered comparisons, strings.Builder / fmt.Stringer
// / user String methods on either side (including MIXED comparisons
// where only one side is a Buffer), and Buffer embeds — must produce no
// diagnostics at all.
func TestPS2045Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2045.Analyzer, "ps2045neg")
}
