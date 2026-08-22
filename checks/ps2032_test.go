package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2032 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers all ten Append* -> Format*/Quote*
// pairs, non-constant bases, parenthesized shells and a parenthesized
// nil, an aliased strconv import reused under its alias, a NAMED string
// target that keeps its conversion wrapper, an alias of the predeclared
// string that sheds it, multiline argument lists, and the pattern nested
// as another call's argument. Advisory shapes (reported, fix withheld)
// are the comment-bearing ones: a comment in the deleted conversion
// shell, on the deleted nil argument (plain and named-wrapper form), and
// trailing inside the shell. Guarded shapes (no report) are a real
// destination buffer, []byte(nil), a SHADOWED nil identifier, the bare
// []byte result (PS2136's direction), string() of a Format* call, a
// type-parameter target, a fake strconv value, and a shadowed string
// identifier. See equiv_PS2032_test.go for the runtime proof of
// byte-identity over adversarial inputs, including panic parity on
// illegal bases.
func TestPS2032(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2032.Analyzer, "ps2032")
}
