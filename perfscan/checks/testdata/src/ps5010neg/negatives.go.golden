package ps5010neg

import (
	"bytes"
	"strings"
	"unicode"
)

type MyString string

type Bytes []byte

// None of these is the case-mapping round-trip pattern: no diagnostics
// at all.
func negatives(s string, ms MyString, b []byte) {
	// The operand is a real byte slice, not a []byte(string) conversion:
	// rewriting would change which value is case-mapped (and b may
	// alias).
	_ = string(bytes.ToUpper(b))

	// A NAMED string operand: strings.ToUpper(ms) would not compile.
	_ = string(bytes.ToUpper([]byte(ms)))

	// A NAMED outer conversion target: the static type would change.
	_ = MyString(bytes.ToLower([]byte(s)))

	// A defined byte-slice conversion is not the predeclared []byte.
	_ = string(bytes.ToTitle(Bytes(s)))

	// The *Special variants take a unicode.SpecialCase — a different
	// shape, deliberately out of scope.
	_ = string(bytes.ToUpperSpecial(unicode.TurkishCase, []byte(s)))
	_ = string(bytes.ToLowerSpecial(unicode.TurkishCase, []byte(s)))
	_ = string(bytes.ToTitleSpecial(unicode.TurkishCase, []byte(s)))

	// ToValidUTF8 is not a case mapping (and takes a second argument).
	_ = string(bytes.ToValidUTF8([]byte(s), []byte("?")))

	// No outer string conversion: the caller genuinely wants bytes.
	_ = bytes.ToUpper([]byte(s))

	// Already the right spelling.
	_ = strings.ToUpper(s)
}

// A same-named method on a value that shadows the package identifier
// never matches: the callee is pinned by type information.
type fakeBytes struct{}

func (fakeBytes) ToUpper(b []byte) []byte { return b }

func shadowedPkg(s string) string {
	bytes := fakeBytes{}
	return string(bytes.ToUpper([]byte(s)))
}
