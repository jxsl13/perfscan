package ps2027neg

import (
	"bytes"
	"fmt"
	"strings"
)

// strings.Builder.String() is zero-copy (it aliases the built bytes) —
// there is no buffer copy to save, so it is out of scope.
func builder(sb strings.Builder) bool {
	return sb.String() == ""
}

// An interface String() (fmt.Stringer) is not bytes.Buffer's method.
func stringer(s fmt.Stringer) bool {
	return s.String() == "" && len(s.String()) > 0
}

// A user-defined String() on another type never matches.
type myThing struct{}

func (myThing) String() string { return "" }

func userType(m myThing) bool {
	return m.String() == ""
}

// Types that merely embed a bytes.Buffer are out of scope: the method
// promotes, but a pointer embed can hide a nil receiver behind a
// non-pointer static type, and a defined wrapper carries its own
// semantics.
type valueEmbed struct{ bytes.Buffer }

type pointerEmbed struct{ *bytes.Buffer }

func embeds(v valueEmbed, p pointerEmbed) {
	_ = v.String() == ""
	_ = len(v.String()) > 0
	_ = p.String() == ""
}

// Comparisons that are not emptiness tests against a constant stay
// untouched: a non-empty literal, a variable, another String() call, a
// non-constant length bound.
func notEmptiness(s string, n int) {
	var buf, other bytes.Buffer
	_ = buf.String() == "x"
	_ = buf.String() == s
	_ = buf.String() == other.String()
	_ = len(buf.String()) == n
	_ = buf.String() > ""
}

// A bare len(buf.String()) used as a value (not compared against a
// constant) is a different pattern and out of scope here.
func bareLen() int {
	var buf bytes.Buffer
	return len(buf.String())
}

// A shadowed len is not the builtin.
func shadowedLen() bool {
	var buf bytes.Buffer
	len := func(string) int { return 0 }
	return len(buf.String()) == 0
}

// Buffer methods other than String, and String used outside a
// comparison, stay untouched.
func otherUses() string {
	var buf bytes.Buffer
	if buf.Len() == 0 {
		return ""
	}
	return buf.String()
}
