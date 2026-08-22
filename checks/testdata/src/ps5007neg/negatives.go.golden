package ps5007neg

import (
	"bytes"
	"strings"
)

// None of these may be reported.
func negatives(s string, b []byte, sub string) {
	_ = strings.Index(s, "ab") // two bytes: the byte scan cannot express it
	_ = strings.Index(s, "é")  // ONE rune but TWO bytes of UTF-8: the length rule is bytes, not runes
	_ = strings.LastIndex(s, "…")
	_ = strings.Index(s, "")     // empty needle: Index(s, "") == 0, inexpressible with IndexByte
	_ = strings.LastIndex(s, "") // LastIndex(s, "") == len(s), same story
	_ = strings.Index(s, sub)    // not a compile-time constant
	_ = strings.LastIndex(s, string(b))
	_ = strings.IndexByte(s, 'z') // already the byte form
	_ = strings.LastIndexByte(s, 'z')
	_ = strings.IndexAny(s, "z") // a SET needle — a different pattern with different multi-byte semantics
	_ = strings.LastIndexAny(s, "z")
	_ = strings.IndexRune(s, 'z')
	_ = strings.Count(s, "z")       // other strings members are out of scope
	_ = bytes.Index(b, []byte("z")) // the bytes twin's needle is []byte, not a constant string
}

// A same-named method on a shadowing identifier is not the stdlib
// function: type information rejects it (its package is not "strings").
type fake struct{}

func (fake) Index(s, sub string) int     { return 0 }
func (fake) LastIndex(s, sub string) int { return 0 }

func shadowed(s string) int {
	strings := fake{}
	return strings.Index(s, "z") + strings.LastIndex(s, "z")
}

// A func value loses the callee shape: the call's Fun is a plain
// identifier, never a strings selector.
func funcValue(s string) int {
	f := strings.Index
	return f(s, "z")
}
