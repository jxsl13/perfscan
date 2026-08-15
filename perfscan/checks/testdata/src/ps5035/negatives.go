package ps5035

import (
	"bytes"
	"strings"
	"unicode"
)

// Left/right-only trims have no TrimLeftSpace/TrimRightSpace
// counterpart — never matched, not even as advisory.
func leftRight(s string) {
	_ = strings.TrimLeftFunc(s, unicode.IsSpace)
	_ = strings.TrimRightFunc(s, unicode.IsSpace)
}

// Any predicate other than the package-level unicode.IsSpace is out:
// a different unicode classifier, a wrapper func (which could do
// anything), a func-typed variable (even one holding unicode.IsSpace —
// the reference is not the bare selector), and a nil predicate (which
// panics at runtime in both spellings, but is not the pattern).
func otherPredicates(s string) {
	_ = strings.TrimFunc(s, unicode.IsDigit)
	wrapper := func(r rune) bool { return unicode.IsSpace(r) }
	_ = strings.TrimFunc(s, wrapper)
	f := unicode.IsSpace
	_ = strings.TrimFunc(s, f)
	_ = strings.TrimFunc(s, nil)
}

// bytes.TrimFunc is out of scope: a []byte result has a capacity and
// aliasing surface the string rewrite argument does not cover (and it
// is PS2016's territory when wrapped in conversions).
func bytesTrim(b []byte) []byte {
	return bytes.TrimFunc(b, unicode.IsSpace)
}

// A shadowed strings identifier does not resolve to the standard
// library's TrimFunc.
type fakeStrings struct{}

func (fakeStrings) TrimFunc(s string, f func(rune) bool) string { return s }

func shadowedStrings(s string) string {
	strings := fakeStrings{}
	return strings.TrimFunc(s, unicode.IsSpace)
}

// A shadowed unicode identifier does not resolve to the standard
// library's IsSpace.
type fakeUnicode struct{ IsSpace func(rune) bool }

func shadowedUnicode(s string) string {
	unicode := fakeUnicode{IsSpace: func(rune) bool { return false }}
	return strings.TrimFunc(s, unicode.IsSpace)
}
