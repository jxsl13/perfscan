package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5050 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. The suite covers slices.Index -> bytes.IndexByte (plain
// and a named byte-slice destination) and slices.Contains -> IndexByte >= 0 in
// every precedence position (bare, negated to < 0, parenthesized under an
// outer ==, and bare under &&). Advisory cases pin the guards: a go/defer
// position and a comment inside the renamed selector. Negatives stay SILENT: a
// non-byte slice and a named byte element type. See equiv_PS5050_test.go for
// the byte-identity proof, ps5050orphan (the fix is withheld rather than
// orphaning slices), and ps5050add (the bytes import is inserted).
func TestPS5050(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5050.Analyzer, "ps5050")
}

// TestPS5050Orphan pins that when slices.Index over a byte slice is the file's
// only slices use, the fix is withheld (advisory) so it never orphans slices.
func TestPS5050Orphan(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5050.Analyzer, "ps5050orphan")
}

// TestPS5050Add pins the import-add path: the fix inserts bytes.
func TestPS5050Add(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5050.Analyzer, "ps5050add")
}
