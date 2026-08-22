package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5037 is an AutoFix check: the fixes are applied and diffed against a
// .go.golden fixture. The suite covers all six emptiness shapes on both
// utf8.RuneCountInString (string) and utf8.RuneCount ([]byte), the
// literal on either side of the operator (mirrored), parenthesized
// literal and call, a hex-spelled 0, a named []byte argument, a named
// string arriving through its explicit conversion, a side-effecting
// argument passing through verbatim, the if-condition and &&-composed
// shapes, a NAMED bool context (safe here — the comparison node is
// untouched), and the aliased-import qualifier vanishing into len. The
// advisory cases pin the withheld-fix guards: a constant string
// argument (len of it would be a compile-time constant), a comment
// inside the replaced callee, and a shadowed builtin len. Negatives pin
// the non-emptiness comparisons (== 1, > 1, >= 0, < 0, named-constant
// and variable comparands), a bound result, arithmetic contexts,
// PS2024's throwaway-conversion sites (silent — PS2024 rewrites the
// call first), the untyped-nil argument (len(nil) does not compile),
// and shadowed utf8/local-function callees. See equiv_PS5037_test.go
// for the runtime proof that every shape is bit-identical.
func TestPS5037(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5037.Analyzer, "ps5037")
}

// TestPS5037Dot pins the dot-import path: the bare callee is replaced
// by len when another unicode/utf8 reference keeps the dot import
// legal, and the fix is withheld (advisory) when the matched call is
// the file's last reference — the fix pipeline never prunes a dot
// import.
func TestPS5037Dot(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5037.Analyzer, "ps5037dot")
}
