package ps2126

import (
	"fmt"
)

const boom = "boom"

// Both calls are rewritten; they are the file's ONLY fmt references, so
// the fix also swaps the orphaned fmt import for errors.
func plainLiteral() error {
	return fmt.Errorf("connection closed") // want `fmt\.Errorf on a constant verb-free message runs the whole fmt printer just to copy the string; errors\.New returns the identical \*errors\.errorString without the printer allocation or format scan`
}

// A const identifier bound to a string is a compile-time constant too:
// the argument text is kept byte-verbatim.
func constIdent() error {
	return fmt.Errorf(boom) // want `fmt\.Errorf on a constant verb-free message runs the whole fmt printer just to copy the string; errors\.New returns the identical \*errors\.errorString without the printer allocation or format scan`
}
