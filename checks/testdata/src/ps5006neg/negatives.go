package ps5006neg

import (
	"bytes"
	"strings"
)

type MyString string

type Bytes []byte

// None of these is the prefix/suffix round-trip pattern: no diagnostics
// at all.
func negatives(s, p string, ms MyString, b, bp []byte) {
	// The operand is a real byte slice, not a []byte(string) conversion:
	// rewriting would change which value is trimmed (and b may alias).
	_ = string(bytes.TrimPrefix(b, []byte(p)))

	// The prefix is a real byte slice: it has no string spelling, so
	// there is nothing to rewrite to.
	_ = string(bytes.TrimPrefix([]byte(s), bp))
	_ = string(bytes.TrimSuffix([]byte(s), bp))

	// A []byte literal prefix with no string source (e.g. invalid UTF-8
	// framing bytes) is never rewritten either.
	_ = string(bytes.TrimPrefix([]byte(s), []byte{0xff}))

	// A NAMED string operand: strings.TrimPrefix(ms, ...) would not
	// compile.
	_ = string(bytes.TrimPrefix([]byte(ms), []byte(p)))

	// A NAMED string prefix: strings.TrimPrefix(..., ms) would not
	// compile either.
	_ = string(bytes.TrimPrefix([]byte(s), []byte(ms)))

	// A NAMED outer conversion target: the static type would change.
	_ = MyString(bytes.TrimPrefix([]byte(s), []byte(p)))

	// A defined byte-slice conversion is not the predeclared []byte —
	// on either side.
	_ = string(bytes.TrimPrefix(Bytes(s), []byte(p)))
	_ = string(bytes.TrimPrefix([]byte(s), Bytes(p)))

	// Trim/TrimLeft/TrimRight take a string cutset — that shape belongs
	// to PS5005, and TrimSpace to PS2012, not PS5006.
	_ = string(bytes.Trim([]byte(s), p))
	_ = string(bytes.TrimLeft([]byte(s), p))
	_ = string(bytes.TrimRight([]byte(s), p))
	_ = string(bytes.TrimSpace([]byte(s)))

	// No outer string conversion: the caller genuinely wants bytes.
	_ = bytes.TrimPrefix([]byte(s), []byte(p))

	// Already the right spelling.
	_ = strings.TrimPrefix(s, p)
	_ = strings.TrimSuffix(s, p)
}

// A same-named method on a value that shadows the package identifier
// never matches: the callee is pinned by type information.
type fakeBytes struct{}

func (fakeBytes) TrimPrefix(b, prefix []byte) []byte { return b }

func shadowedPkg(s string) string {
	bytes := fakeBytes{}
	return string(bytes.TrimPrefix([]byte(s), []byte("#")))
}
