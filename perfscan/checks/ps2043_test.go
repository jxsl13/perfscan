package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS2043 is an AutoFix check: the fixes are applied and diffed against a
// .go.golden fixture. The suite covers the positives (encoding/hex's
// package-level EncodeToString, an aliased hex import, every stock
// base64/base32 encoder plus a derived WithPadding receiver, a local
// *Encoding and an addressable Encoding VALUE receiver, a call/named/conversion
// argument, an arithmetic parent and a parenthesized inner call), the advisory
// guards (an untyped nil argument — len(nil) does not compile — and comments
// inside the rewritten scaffolding), and the negatives (stored-then-measured,
// a used result, a method value, a same-named method on another type, a
// wrapper EMBEDDING *base64.Encoding whose own EncodedLen would shadow the
// promoted one, a shadowed len, a shadowed hex, and DecodeString). See
// equiv_PS2043_test.go for the runtime proof that
// len(EncodeToString(b)) == EncodedLen(len(b)) for every encoder variant over
// adversarial inputs, plus the pinned divergence witness behind the embedding
// guard.
func TestPS2043(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS2043.Analyzer, "ps2043")
}
