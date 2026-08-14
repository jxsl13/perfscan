package ps5011neg

import (
	"bytes"
	"strings"
)

type MyString string

type Bytes []byte

// None of these is the replace round-trip pattern: no diagnostics at
// all.
func negatives(s, old, new string, ms MyString, b, bo, bn []byte, n int) {
	// The subject is a real byte slice, not a []byte(string) conversion:
	// rewriting would change which value is scanned (and b may alias).
	_ = string(bytes.ReplaceAll(b, []byte(old), []byte(new)))

	// old or new is a real byte slice: it has no string spelling, so
	// there is nothing to rewrite to.
	_ = string(bytes.ReplaceAll([]byte(s), bo, []byte(new)))
	_ = string(bytes.ReplaceAll([]byte(s), []byte(old), bn))
	_ = string(bytes.Replace([]byte(s), bo, bn, n))

	// A []byte literal with no string source (e.g. invalid UTF-8 framing
	// bytes) is never rewritten either.
	_ = string(bytes.ReplaceAll([]byte(s), []byte{0xff}, []byte(new)))

	// A NAMED string operand in ANY position: the strings twin would not
	// compile.
	_ = string(bytes.ReplaceAll([]byte(ms), []byte(old), []byte(new)))
	_ = string(bytes.ReplaceAll([]byte(s), []byte(ms), []byte(new)))
	_ = string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(ms)))

	// A NAMED outer conversion target: the static type would change.
	_ = MyString(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new)))

	// A defined byte-slice conversion is not the predeclared []byte — in
	// any position.
	_ = string(bytes.ReplaceAll(Bytes(s), []byte(old), []byte(new)))
	_ = string(bytes.ReplaceAll([]byte(s), Bytes(old), []byte(new)))
	_ = string(bytes.ReplaceAll([]byte(s), []byte(old), Bytes(new)))

	// Map takes a mapping func — out of scope.
	_ = string(bytes.Map(func(r rune) rune { return r }, []byte(s)))

	// No outer string conversion: the caller genuinely wants bytes.
	_ = bytes.ReplaceAll([]byte(s), []byte(old), []byte(new))
	_ = bytes.Replace([]byte(s), []byte(old), []byte(new), n)

	// Already the right spelling.
	_ = strings.ReplaceAll(s, old, new)
	_ = strings.Replace(s, old, new, n)
}

// A same-named method on a value that shadows the package identifier
// never matches: the callee is pinned by type information.
type fakeBytes struct{}

func (fakeBytes) ReplaceAll(s, old, new []byte) []byte { return s }

func shadowedPkg(s string) string {
	bytes := fakeBytes{}
	return string(bytes.ReplaceAll([]byte(s), []byte("#"), []byte("%")))
}
