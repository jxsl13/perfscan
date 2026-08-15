package ps3031

import (
	"bytes"
	"strings"
	"unicode"
)

// Left/right-only trims have no TrimLeftSpace/TrimRightSpace
// counterpart — never matched, not even as advisory.
func leftRight(b []byte) {
	_ = bytes.TrimLeftFunc(b, unicode.IsSpace)
	_ = bytes.TrimRightFunc(b, unicode.IsSpace)
}

// Any predicate other than the package-level unicode.IsSpace is out:
// a different unicode classifier, a wrapper func (which could do
// anything), a func-typed variable (even one holding unicode.IsSpace —
// the reference is not the bare selector), and a nil predicate (which
// panics at runtime in both spellings, but is not the pattern).
func otherPredicates(b []byte) {
	_ = bytes.TrimFunc(b, unicode.IsDigit)
	wrapper := func(r rune) bool { return unicode.IsSpace(r) }
	_ = bytes.TrimFunc(b, wrapper)
	f := unicode.IsSpace
	_ = bytes.TrimFunc(b, f)
	_ = bytes.TrimFunc(b, nil)
}

// strings.TrimFunc is PS5035's site, never this check's.
func stringsTrim(s string) string {
	return strings.TrimFunc(s, unicode.IsSpace)
}

// A shadowed bytes identifier does not resolve to the standard
// library's TrimFunc.
type fakeBytes struct{}

func (fakeBytes) TrimFunc(b []byte, f func(rune) bool) []byte { return b }

func shadowedBytes(b []byte) []byte {
	bytes := fakeBytes{}
	return bytes.TrimFunc(b, unicode.IsSpace)
}

// A shadowed unicode identifier does not resolve to the standard
// library's IsSpace.
type fakeUnicode struct{ IsSpace func(rune) bool }

func shadowedUnicode(b []byte) []byte {
	unicode := fakeUnicode{IsSpace: func(rune) bool { return false }}
	return bytes.TrimFunc(b, unicode.IsSpace)
}
