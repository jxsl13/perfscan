package ps2016neg

import (
	"bytes"
	"strings"
	"unicode"
)

type MyString string

type Bytes []byte

// None of these is the predicate round-trip pattern: no diagnostics at
// all.
func negatives(s string, ms MyString, b []byte, f func(rune) bool) {
	// The operand is a real byte slice, not a []byte(string) conversion:
	// rewriting would change which value is trimmed (and b may alias).
	_ = string(bytes.TrimFunc(b, f))

	// A NAMED string operand: strings.TrimFunc(ms, f) would not compile.
	_ = string(bytes.TrimFunc([]byte(ms), f))

	// A NAMED outer conversion target: the static type would change.
	_ = MyString(bytes.TrimFunc([]byte(s), f))

	// A defined byte-slice conversion is not the predeclared []byte.
	_ = string(bytes.TrimFunc(Bytes(s), f))

	// The cutset-taking Trim family is PS5005's shape, not PS2016's.
	_ = string(bytes.Trim([]byte(s), " "))
	_ = string(bytes.TrimLeft([]byte(s), " "))
	_ = string(bytes.TrimRight([]byte(s), " "))

	// The predicate-free TrimSpace shape belongs to PS2012.
	_ = string(bytes.TrimSpace([]byte(s)))

	// IndexFunc/LastIndexFunc return offsets — a different shape.
	_ = bytes.IndexFunc([]byte(s), f)
	_ = bytes.LastIndexFunc([]byte(s), f)

	// No outer string conversion: the caller genuinely wants bytes.
	_ = bytes.TrimFunc([]byte(s), unicode.IsSpace)

	// Already the right spelling.
	_ = strings.TrimFunc(s, f)
}

// A same-named method on a value that shadows the package identifier
// never matches: the callee is pinned by type information.
type fakeBytes struct{}

func (fakeBytes) TrimFunc(b []byte, f func(rune) bool) []byte { return b }

func shadowedPkg(s string) string {
	bytes := fakeBytes{}
	return string(bytes.TrimFunc([]byte(s), unicode.IsSpace))
}
