package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5051 is an AutoFix check: fixes are applied and diffed against a
// .go.golden fixture. The suite covers FormatInt == / != and FormatUint == in
// several bases, expression arguments that unwrap without parentheses, a
// comment-inside advisory, and negatives that stay SILENT: an ordering
// comparison, mismatched bases, a FormatInt/FormatUint mix, a non-constant
// base, and an out-of-range base. See equiv_PS5051_test.go for the
// byte-identity proof and ps5051orphan (the fix is withheld rather than
// orphaning strconv).
func TestPS5051(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5051.Analyzer, "ps5051")
}

// TestPS5051Orphan pins that when the FormatInt calls are the file's only
// strconv use, the fix is withheld (advisory) so it never orphans strconv.
func TestPS5051Orphan(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5051.Analyzer, "ps5051orphan")
}
