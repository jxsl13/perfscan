package ps2018neg

import (
	"bytes"
	"strings"
)

type MyString string

type Bytes []byte

// None of these is the string-result Repeat round-trip pattern: no
// diagnostics at all.
func negatives(s string, ms MyString, b []byte) {
	// The seed is a real byte slice, not a []byte(string) conversion:
	// rewriting would change which value is repeated (and b may alias).
	_ = string(bytes.Repeat(b, 4))

	// A NAMED string seed: strings.Repeat(ms, 4) would not compile.
	_ = string(bytes.Repeat([]byte(ms), 4))

	// A NAMED outer conversion target: the static type would change.
	_ = MyString(bytes.Repeat([]byte(s), 4))

	// A defined byte-slice conversion is not the predeclared []byte.
	_ = string(bytes.Repeat(Bytes(s), 4))

	// No outer string conversion: the caller genuinely wants bytes.
	_ = bytes.Repeat([]byte(s), 4)

	// Already the right spelling.
	_ = strings.Repeat(s, 4)

	// A different bytes function is a different pattern.
	_ = string(bytes.ToUpper([]byte(s)))
}

// A same-named method on a value that shadows the package identifier
// never matches: the callee is pinned by type information.
type fakeBytes struct{}

func (fakeBytes) Repeat(b []byte, n int) []byte { return b }

func shadowedPkg(s string) string {
	bytes := fakeBytes{}
	return string(bytes.Repeat([]byte(s), 4))
}
