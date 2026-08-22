package ps2012neg

import (
	"bytes"
	"strings"
)

type MyString string

type Bytes []byte

// None of these is the round-trip pattern: no diagnostics at all.
func negatives(s string, ms MyString, b []byte) {
	// The operand is a real byte slice, not a []byte(string) conversion:
	// rewriting would change which value is trimmed (and b may alias).
	_ = string(bytes.TrimSpace(b))

	// A NAMED string operand: strings.TrimSpace(ms) would not compile.
	_ = string(bytes.TrimSpace([]byte(ms)))

	// A NAMED outer conversion target: the static type would change.
	_ = MyString(bytes.TrimSpace([]byte(s)))

	// A defined byte-slice conversion is not the predeclared []byte.
	_ = string(bytes.TrimSpace(Bytes(s)))

	// Other trims take a cutset — out of scope by design.
	_ = string(bytes.TrimLeft([]byte(s), " "))
	_ = string(bytes.Trim([]byte(s), " \t"))

	// No outer string conversion: the caller genuinely wants bytes.
	_ = bytes.TrimSpace([]byte(s))

	// Already the right spelling.
	_ = strings.TrimSpace(s)
}

// A same-named method on a value that shadows the package identifier
// never matches: the callee is pinned by type information.
type fakeBytes struct{}

func (fakeBytes) TrimSpace(b []byte) []byte { return b }

func shadowedPkg(s string) string {
	bytes := fakeBytes{}
	return string(bytes.TrimSpace([]byte(s)))
}
