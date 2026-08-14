package ps2137

import (
	"fmt"
)

// fmt.Sprint over a plain int: fixed to strconv.Itoa.
func sprintInt(n int) string {
	return fmt.Sprint(n) // want `fmt\.Sprint\(i\) / fmt\.Sprintf\("%v", i\) on an integer pays fmt's reflection and boxing for a plain decimal; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// fmt.Sprintf("%v", ...) over a plain int: fixed to strconv.Itoa.
func sprintfVInt(n int) string {
	return fmt.Sprintf("%v", n) // want `fmt\.Sprint\(i\) / fmt\.Sprintf\("%v", i\) on an integer pays fmt's reflection and boxing for a plain decimal; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// A signed non-int width: fixed to strconv.FormatInt with the int64 widening.
func sprintInt64(n int64) string {
	return fmt.Sprint(n) // want `fmt\.Sprint\(i\) / fmt\.Sprintf\("%v", i\) on an integer pays fmt's reflection and boxing for a plain decimal; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// An unsigned width: fixed to strconv.FormatUint with the uint64 widening.
func sprintUint(u uint) string {
	return fmt.Sprintf("%v", u) // want `fmt\.Sprint\(i\) / fmt\.Sprintf\("%v", i\) on an integer pays fmt's reflection and boxing for a plain decimal; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// A string operand is the IDENTITY, owned by PS2130 — silent here.
func sprintString(s string) string {
	return fmt.Sprint(s)
}

// "%d" is PS2107's territory — silent here.
func sprintfD(n int) string {
	return fmt.Sprintf("%d", n)
}

// THE CRITICAL GUARD: %v and fmt.Sprint HONOR fmt.Stringer. MyInt has a
// String() method, so fmt prints "MyInt(7)" — NOT the decimal digits — and
// a strconv rewrite would change the output. Named types must stay SILENT
// (not even advisory: the message would be wrong for them).
type MyInt int

func (m MyInt) String() string { return "MyInt" }

func sprintStringer(m MyInt) string {
	return fmt.Sprint(m)
}

// A named integer WITHOUT methods is silent too — the unnamed-only guard
// does not depend on whether a String method exists today.
type port int

func sprintNamed(p port) string {
	return fmt.Sprintf("%v", p)
}

// Two operands: Sprint would need its operand handling (and %v twice would
// be a different format) — silent.
func sprintPair(a, b int) string {
	return fmt.Sprint(a, b)
}

// A float operand is not an integer (its decimal form is FormatFloat's
// business, with entirely different formatting) — silent.
func sprintFloat(f float64) string {
	return fmt.Sprint(f)
}

// A variadic spread is not a single operand — silent.
func sprintSpread(xs []any) string {
	return fmt.Sprint(xs...)
}

// Literal text around the verb is not a bare conversion — silent.
func sprintfLabeled(n int) string {
	return fmt.Sprintf("id=%v", n)
}

// strconv is shadowed at the call site: the rewrite could not reference
// the package — reported, no fix.
func shadowed(n int) string {
	strconv := n
	_ = strconv
	return fmt.Sprint(n) // want `fmt\.Sprint\(i\) / fmt\.Sprintf\("%v", i\) on an integer pays fmt's reflection and boxing for a plain decimal; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

// COMMENT-SAFETY: a comment inside the replaced range would be silently
// deleted by the rewrite — reported, no fix, call left intact.
func commentInCall(n int) string {
	return fmt.Sprint(n /* the id */) // want `fmt\.Sprint\(i\) / fmt\.Sprintf\("%v", i\) on an integer pays fmt's reflection and boxing`
}
