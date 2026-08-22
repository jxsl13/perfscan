package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS3031 is an AutoFix check: the fixes are applied and diffed against a
// .go.golden fixture. The suite covers the canonical call, verbatim
// source arguments (field selector, side-effecting compound expression,
// a named []byte type, a parenthesized predicate), a nested match inside
// the kept source span, a multi-line call, a surviving comment BEFORE
// the source argument, the orphaned-unicode-import drop, aliased bytes
// and unicode imports, and a file whose other unicode reference keeps
// the import. The advisory fixtures pin the withheld-fix guards: a
// comment inside the deleted span (with a fixable site alongside that
// must then NOT drop the still-referenced import) and a unicode path
// imported under two specs (dropping one would still leave a compile
// error). The negatives pin what never matches at all: TrimLeftFunc /
// TrimRightFunc (no TrimLeftSpace/TrimRightSpace exists), any predicate
// but the bare package-level unicode.IsSpace selector (another
// classifier, a wrapper func, a func-typed variable holding
// unicode.IsSpace, nil), strings.TrimFunc (PS5035's site), and shadowed
// bytes / unicode identifiers. See equiv_PS3031_test.go for the runtime
// proof that bytes.TrimSpace and bytes.TrimFunc(b, unicode.IsSpace)
// return results identical in bytes, data pointer, length, capacity,
// and nil-ness on every input.
func TestPS3031(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS3031.Analyzer, "ps3031")
}
