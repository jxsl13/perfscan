package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5023 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers strings.IndexRune and
// bytes.IndexRune with constant ASCII runes over every literal spelling
// (plain runes, escape sequences, NUL, the 0x7f upper bound, \u and octal
// escapes of ASCII code points, plain integer literals), the
// rename-only fix in every syntactic position (parenthesized arguments
// and calls, conditions, index expressions, go/defer — the replacement
// stays a call, so nothing else is ever edited), the aliased-import
// qualifiers, and the advisory guard (named-constant and
// constant-expression runes stay unrewritten — a typed rune constant
// would need an inserted byte(...) conversion). See equiv_PS5023_test.go
// for the runtime proof that the rewrite returns the identical index on
// every input — and for the divergence witnesses pinning why the
// constant-only and [0, 0x80) bounds are load-bearing.
func TestPS5023(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5023.Analyzer, "ps5023")
}

// Negative shapes — non-constant runes (the crucial truncation hazard),
// non-ASCII and negative and beyond-range constants, utf8.RuneError, the
// ContainsRune/IndexByte/substring/cutset siblings owned by other checks,
// a shadowing identifier, a func value, and a dot import — must produce
// no diagnostics at all.
func TestPS5023Negatives(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5023.Analyzer, "ps5023neg")
}
