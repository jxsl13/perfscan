package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS3030 is an AutoFix check: the fixes are applied and diffed against
// .go.golden fixtures. The suite covers the basic call and the assigned
// result, an aliased bytes qualifier (kept verbatim — only the selected
// name changes), an aliased unicode predicate qualifier (whose import
// survives thanks to another reference), a parenthesized predicate, a
// side-effecting b argument passing through byte-verbatim, the
// orphaned-unicode-import removal when the fixes delete a file's last
// unicode references, and the mixed file where an advisory site's
// surviving reference correctly blocks that removal. Advisory case: a
// comment inside the deleted ", unicode.IsSpace)" scaffolding withholds
// the fix. The negatives pin the guards: an equivalent wrapper literal,
// any other predicate, a variable holding unicode.IsSpace,
// strings.FieldsFunc (PS5034's territory), and shadowed bytes / unicode
// identifiers. See equiv_PS3030_test.go for the runtime proof that
// Fields and FieldsFunc(b, unicode.IsSpace) agree byte-for-byte (len,
// cap, non-nil-ness, elements, and per-field backing-array identity) on
// every input.
func TestPS3030(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3030.Analyzer, "ps3030")
}
