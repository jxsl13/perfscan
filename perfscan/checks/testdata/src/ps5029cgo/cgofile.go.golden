package ps5029cgo

// The only fmt reference in this cgo FILE is the fixable Sprintln
// itself: the concatenation rewrite would orphan the import, and a cgo
// file's import block is never pruned, so the fix is withheld — the
// report stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import "fmt"

func cgoConcat(a, b string) string {
	return fmt.Sprintln(a, b) // want `fmt\.Sprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's reflection machinery; direct \+ concatenation with " " and "\\n" builds the identical string`
}
