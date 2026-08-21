package ps5005neg

import (
	"bytes"
	"strings"
)

type MyString string

type Bytes []byte

// None of these is the cutset round-trip pattern: no diagnostics at all.
func negatives(s, cutset string, ms MyString, b, bcut []byte) {
	// The operand is a real byte slice, not a []byte(string) conversion:
	// rewriting would change which value is trimmed (and b may alias).
	_ = string(bytes.Trim(b, cutset))

	// A NAMED string operand: strings.Trim(ms, cutset) would not compile.
	_ = string(bytes.Trim([]byte(ms), cutset))

	// A NAMED outer conversion target: the static type would change.
	_ = MyString(bytes.Trim([]byte(s), cutset))

	// A defined byte-slice conversion is not the predeclared []byte.
	_ = string(bytes.Trim(Bytes(s), cutset))

	// TrimPrefix/TrimSuffix take a []byte second argument — a different
	// shape, deliberately out of scope.
	_ = string(bytes.TrimPrefix([]byte(s), bcut))
	_ = string(bytes.TrimSuffix([]byte(s), bcut))

	// The cutset-free TrimSpace shape belongs to PS2012, not PS5005.
	_ = string(bytes.TrimSpace([]byte(s)))

	// TrimFunc takes a predicate — out of scope.
	_ = string(bytes.TrimFunc([]byte(s), func(rune) bool { return false }))

	// No outer string conversion: the caller genuinely wants bytes.
	_ = bytes.Trim([]byte(s), cutset)

	// Already the right spelling.
	_ = strings.Trim(s, cutset)
}

// A same-named method on a value that shadows the package identifier
// never matches: the callee is pinned by type information.
type fakeBytes struct{}

func (fakeBytes) Trim(b []byte, cutset string) []byte { return b }

func shadowedPkg(s string) string {
	bytes := fakeBytes{}
	return string(bytes.Trim([]byte(s), " "))
}
