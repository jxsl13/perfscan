package ps2017neg

import (
	"bytes"
	"strings"
	"unicode"
)

type MyString string

type Bytes []byte

// None of these is the Map round-trip pattern: no diagnostics at all.
func negatives(s string, ms MyString, b []byte) {
	// The operand is a real byte slice, not a []byte(string) conversion:
	// rewriting would change which value is mapped (and b may alias).
	_ = string(bytes.Map(unicode.ToUpper, b))

	// A NAMED string operand: strings.Map(f, ms) would not compile.
	_ = string(bytes.Map(unicode.ToUpper, []byte(ms)))

	// A NAMED outer conversion target: the static type would change.
	_ = MyString(bytes.Map(unicode.ToLower, []byte(s)))

	// A defined byte-slice conversion is not the predeclared []byte.
	_ = string(bytes.Map(unicode.ToTitle, Bytes(s)))

	// No outer string conversion: the caller genuinely wants bytes.
	_ = bytes.Map(unicode.ToUpper, []byte(s))

	// Already the right spelling.
	_ = strings.Map(unicode.ToUpper, s)
}

// A same-named method on a value that shadows the package identifier
// never matches: the callee is pinned by type information.
type fakeBytes struct{}

func (fakeBytes) Map(f func(rune) rune, b []byte) []byte { return b }

func shadowedPkg(s string) string {
	bytes := fakeBytes{}
	return string(bytes.Map(unicode.ToUpper, []byte(s)))
}
