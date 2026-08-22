package ps2045neg

import (
	"bytes"
	"fmt"
	"strings"
)

// A constant far operand is PS2031's pattern (bytes.Equal against
// []byte(lit)) — PS2045 must stay silent on it, in both operand
// orders, as on any non-call operand.
func oneSideNotACall(buf bytes.Buffer, s string) {
	_ = buf.String() == "OK"
	_ = "OK" != buf.String()
	_ = buf.String() == s
	_ = s == buf.String()
}

// Ordered comparisons cannot be expressed with bytes.Equal.
func ordered(a, b bytes.Buffer) {
	_ = a.String() < b.String()
	_ = a.String() >= b.String()
}

// Same-named methods on other types never match: strings.Builder, a
// fmt.Stringer interface, a user-defined String — on BOTH sides or,
// crucially, MIXED with a real bytes.Buffer (only the Buffer side has
// a zero-copy byte view; the other side would need a conversion copy).
type myStringer struct{}

func (myStringer) String() string { return "OK" }

func otherTypes(buf bytes.Buffer, sb strings.Builder, st fmt.Stringer, my myStringer) {
	_ = sb.String() == st.String()
	_ = my.String() != my.String()
	_ = buf.String() == sb.String()
	_ = sb.String() == buf.String()
	_ = buf.String() != st.String()
	_ = buf.String() == my.String()
}

// Types that merely embed a bytes.Buffer are out of scope: the
// receiver's static type is the outer type, and a pointer embed can
// hide a nil.
type valueEmbed struct{ bytes.Buffer }

type pointerEmbed struct{ *bytes.Buffer }

func embeds(v valueEmbed, p pointerEmbed, buf bytes.Buffer) {
	_ = v.String() == buf.String()
	_ = p.String() == buf.String()
	_ = v.String() != p.String()
}

// Unrelated Buffer expressions that must stay silent.
func unrelated(a, b bytes.Buffer) {
	_ = a.Len() == b.Len()
	_ = string(a.Bytes()) == string(b.Bytes()) // conversions, not Buffer.String — gc elides these copies
	_ = bytes.Equal(a.Bytes(), b.Bytes())      // already the After shape
}
