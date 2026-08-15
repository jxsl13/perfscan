package ps2031neg

// Negative shapes: none of these may be reported, let alone rewritten.

import (
	"errors"
	"fmt"
)

// Anything beyond the single bare verb formats differently — width,
// flags, index, other verbs, extra text, an escaped percent.
func decorated(msg string) []error {
	return []error{
		fmt.Errorf("%s\n", msg),
		fmt.Errorf("%q", msg),
		fmt.Errorf("%d", 42),
		fmt.Errorf("%+v", msg),
		fmt.Errorf("%5s", msg),
		fmt.Errorf("%[1]s", msg),
		fmt.Errorf("%%s", msg),
		fmt.Errorf("x: %s", msg),
	}
}

// A variable or named constant format proves nothing at the call site —
// only a literal makes the vanishing format locally evident.
const fmtConst = "%s"

func nonLiteral(msg string) (error, error) {
	f := "%s"
	return fmt.Errorf(f, msg), fmt.Errorf(fmtConst, msg)
}

// A defined string type is not the predeclared string: substituting it
// into errors.New would side-step fmt's formatting of the named type
// (same static-type guard as PS2130).
type myStr string

func defined(m myStr) error {
	return fmt.Errorf("%s", m)
}

// []byte and error operands are not verbatim string pass-throughs.
func notString(b []byte, err error) (error, error) {
	return fmt.Errorf("%s", b), fmt.Errorf("%v", err)
}

// A Stringer routes through String().
type stringy struct{}

func (stringy) String() string { return "stringy" }

func stringer(s stringy) error {
	return fmt.Errorf("%s", s)
}

// %w wraps: fmt returns a *wrapError with Unwrap, never errors.New.
func wrapped(err error) error {
	return fmt.Errorf("%w", err)
}

// Wrong arity or a variadic spread.
func arity(msg string, args []any) []error {
	return []error{
		fmt.Errorf("plain message"),
		fmt.Errorf("%s %s", msg, msg),
		fmt.Errorf("%s", args...),
	}
}

// A shadowed fmt is not the fmt package.
type fakeFmt struct{}

func (fakeFmt) Errorf(format string, a ...any) error { return errors.New(format) }

func shadowed(msg string) error {
	fmt := fakeFmt{}
	return fmt.Errorf("%s", msg)
}

// fmt.Sprintf with the same shape returns a string, not an error —
// that identity is PS2130's territory.
func sprintf(msg string) string {
	return fmt.Sprintf("%s", msg)
}
