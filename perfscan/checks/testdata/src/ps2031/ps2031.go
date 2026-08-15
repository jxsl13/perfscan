package ps2031

import (
	"errors"
	"fmt"
)

// The basic shapes: a bare %s or %v verb on a plain string operand.
func basic(msg string) error {
	return fmt.Errorf("%s", msg) // want `fmt\.Errorf with a bare %s verb`
}

func verbV(msg string) error {
	return fmt.Errorf("%v", msg) // want `fmt\.Errorf with a bare %v verb`
}

// The operand text stays byte-verbatim in place: a concatenation keeps
// its spelling, and errors.New's parentheses already delimit it.
func concat(a, b string) error {
	return fmt.Errorf("%s", a+" / "+b) // want `fmt\.Errorf with a bare %s verb`
}

// A call operand is evaluated exactly once on both sides.
func callArg(f func() string) error {
	return fmt.Errorf("%v", f()) // want `fmt\.Errorf with a bare %v verb`
}

// Selector and index operands carry over untouched.
type box struct{ msg string }

func selIdx(bx box, parts []string) (error, error) {
	e1 := fmt.Errorf("%s", bx.msg)   // want `fmt\.Errorf with a bare %s verb`
	e2 := fmt.Errorf("%s", parts[0]) // want `fmt\.Errorf with a bare %s verb`
	return e1, e2
}

// A raw-string format and a parenthesized format literal both match;
// the whole format region is deleted outright.
func rawParen(msg string) (error, error) {
	e1 := fmt.Errorf(`%s`, msg)   // want `fmt\.Errorf with a bare %s verb`
	e2 := fmt.Errorf(("%v"), msg) // want `fmt\.Errorf with a bare %v verb`
	return e1, e2
}

// '%' inside the OPERAND is data under %s/%v — never re-parsed; the
// rewrite keeps it byte-verbatim (an untyped string constant defaults
// to string and matches).
func percentData() error {
	return fmt.Errorf("%v", "100% sure, %s and %w included") // want `fmt\.Errorf with a bare %v verb`
}

// A comment INSIDE the operand survives — the operand bytes are not
// touched; only the wrapper around it is rewritten.
func commentInOperand(a, b string) error {
	return fmt.Errorf("%s", a+ /* mid */ b) // want `fmt\.Errorf with a bare %s verb`
}

// A comment overlapping the deleted wrapper text would be silently
// destroyed — reported but NOT fixed.
func commentedFormat(msg string) error {
	return fmt.Errorf( /* keep me */ "%s", msg) // want `fmt\.Errorf with a bare %s verb`
}

func commentedClose(msg string) error {
	return fmt.Errorf("%s", msg /* trailing */) // want `fmt\.Errorf with a bare %s verb`
}

// A local name shadowing the errors package leaves errors.New
// unspellable at the call site — reported but NOT fixed.
func shadowedErrors(msg string) error {
	errors := 3
	_ = errors
	return fmt.Errorf("%s", msg) // want `fmt\.Errorf with a bare %s verb`
}

// Keep fmt referenced so this file's import is not orphaned (the orphan
// path is exercised by orphan.go).
func other() {
	fmt.Println("side effect")
}

// Keep errors referenced independently of the fixes.
var _ = errors.New
