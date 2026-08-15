package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5034 is an AutoFix check: the fixes are applied and diffed against a
// .go.golden fixture. The suite covers the canonical call, verbatim
// source arguments (field selector, side-effecting compound expression,
// a named string type, a parenthesized predicate), a nested match inside
// the kept source span, a multi-line call, a surviving comment BEFORE
// the source argument, the orphaned-unicode-import drop, aliased strings
// and unicode imports, and a file whose other unicode reference keeps
// the import. The advisory fixtures pin the withheld-fix guards: a
// comment inside the deleted span (with a fixable site alongside that
// must then NOT drop the still-referenced import) and a unicode path
// imported under two specs (dropping one would still leave a compile
// error). The negatives pin what never matches at all: TrimLeftFunc /
// TrimRightFunc (no TrimLeftSpace/TrimRightSpace exists), any predicate
// but the bare package-level unicode.IsSpace selector (another
// classifier, a wrapper func, a func-typed variable holding
// unicode.IsSpace, nil), bytes.TrimFunc, and shadowed strings / unicode
// identifiers. See equiv_PS5034_test.go for the runtime proof that
// TrimSpace and TrimFunc(s, unicode.IsSpace) return byte-identical
// results on every input.
func TestPS5034(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5034.Analyzer, "ps5034")
}
