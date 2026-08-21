package ps5029

// The only fmt reference in this FILE is the fixable Sprintln itself:
// the concatenation rewrite orphans the import, and the fix pipeline
// prunes the now-unused fmt import afterwards, so the fix applies
// (non-cgo file).

import (
	"fmt"
)

func orphanConcat(a, b string) string {
	return fmt.Sprintln(a, b) // want `fmt\.Sprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's reflection machinery; direct \+ concatenation with " " and "\\n" builds the identical string`
}
