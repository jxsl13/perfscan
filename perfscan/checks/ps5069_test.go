package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5069 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers each recognized predicate
// (HasPrefix, HasSuffix, Contains, EqualFold, Index, LastIndex, Count),
// the provably-non-nil receiver shapes (a value bytes.Buffer, &buf,
// new(bytes.Buffer)), and a named string constant that carries verbatim
// into the []byte conversion. The advisory guards live in ps5069adv: a
// not-provably-non-nil *bytes.Buffer, a shadowed bytes identifier with
// no usable qualifier, and comment-bearing syntax. See
// equiv_PS5069_test.go for the runtime proof that Before and After agree
// over every buffer state — and for the nil-receiver divergence that
// keeps pointer receivers advisory.
func TestPS5069(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5069.Analyzer, "ps5069", "ps5069adv")
}

// Negative shapes — a non-constant needle, the empty string, a
// strings.Builder receiver, an unrecognized strings function, ContainsAny
// (string needle), and a plain-string first argument — must produce no
// diagnostics at all.
func TestPS5069Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5069.Analyzer, "ps5069neg")
}
