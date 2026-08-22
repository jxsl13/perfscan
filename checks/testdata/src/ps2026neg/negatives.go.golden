package ps2026neg

import (
	"bytes"
	"fmt"
	"strings"
)

// strings.Builder.String is a ZERO-COPY unsafe cast — len(sb.String())
// allocates nothing, so the bytes.Buffer rationale does not apply and
// the check never matches another package's String.
func builder(sb *strings.Builder) int {
	return len(sb.String())
}

// A same-named method on another type never matches: the callee is
// pinned to the standard library's (*bytes.Buffer).String by type info.
type stringer struct{}

func (stringer) String() string { return "custom" }

func ownString(s stringer) int {
	return len(s.String())
}

// An interface call is fmt.Stringer's String, not bytes.Buffer's: the
// dynamic type may have any String semantics, so it never matches.
func viaInterface(s fmt.Stringer) int {
	return len(s.String())
}

// A shadowed len resolves to some other object and is rejected.
func shadowedLen(buf bytes.Buffer) int {
	len := func(string) int { return 0 }
	return len(buf.String())
}

// A method promoted through an embedded field is out of scope: the
// outer type may define its own Len, and a pointer embedding may be
// nil (String would nil-guard where Len panics).
type valueWrap struct{ bytes.Buffer }

type ptrWrap struct{ *bytes.Buffer }

func promoted(v valueWrap, p ptrWrap) {
	_ = len(v.String())
	_ = len(p.String())
}

// A method VALUE severs the receiver from the call site: the inner call
// is not a selector call, so it never matches.
func methodValue(buf bytes.Buffer) int {
	f := buf.String
	return len(f())
}

// The already-direct spelling and unrelated len calls stay silent.
func alreadyDirect(buf bytes.Buffer, bufs []bytes.Buffer) {
	_ = buf.Len()
	_ = len("x")
	_ = len(bufs)
}

// String() used for its VALUE (not just its length) is a different
// pattern and out of scope.
func stringUsed(buf bytes.Buffer) (string, int) {
	s := buf.String()
	return s, len(s)
}

// A type parameter's String comes from the local constraint interface,
// not from package bytes.
type hasString interface{ String() string }

func generic[T hasString](v T) int {
	return len(v.String())
}
