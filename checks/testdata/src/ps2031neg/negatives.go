package ps2031neg

import (
	"bytes"
	"fmt"
	"strings"
)

// The empty string is PS2027's pattern (buf.Len() == 0 is stronger) —
// PS2031 must stay silent on it, in both operand orders.
func emptyString(buf bytes.Buffer) {
	_ = buf.String() == ""
	_ = buf.String() != ""
	_ = "" == buf.String()
}

// Emptiness via len is PS2027's pattern too.
func lenForm(buf bytes.Buffer) {
	_ = len(buf.String()) == 2
}

// A non-constant far operand is out of scope: []byte(s) of a
// non-constant string would trade the buffer copy for an operand copy.
func nonConstant(buf, buf2 bytes.Buffer, s string) {
	_ = buf.String() == s
	_ = s != buf.String()
	_ = buf.String() == buf2.String()
	_ = buf.String() == fmt.Sprint("OK")
}

// Ordered comparisons cannot be expressed with bytes.Equal.
func ordered(buf bytes.Buffer) {
	_ = buf.String() < "OK"
	_ = buf.String() >= "OK"
}

// Same-named methods on other types never match: strings.Builder, a
// fmt.Stringer interface, a user-defined String.
type myStringer struct{}

func (myStringer) String() string { return "OK" }

func otherTypes(sb strings.Builder, st fmt.Stringer, my myStringer) {
	_ = sb.String() == "OK"
	_ = st.String() == "OK"
	_ = my.String() == "OK"
}

// Types that merely embed a bytes.Buffer are out of scope: the
// receiver's static type is the outer type, and a pointer embed can
// hide a nil.
type valueEmbed struct{ bytes.Buffer }

type pointerEmbed struct{ *bytes.Buffer }

func embeds(v valueEmbed, p pointerEmbed) {
	_ = v.String() == "OK"
	_ = p.String() == "OK"
}

// A defined (non-alias) type on Buffer has no String method of its
// own; converting through it is unrelated code that must stay silent.
func unrelated(buf bytes.Buffer) {
	_ = buf.Len() == 2
	_ = string(buf.Bytes()) == "OK" // a conversion, not Buffer.String — gc already elides this copy
}
