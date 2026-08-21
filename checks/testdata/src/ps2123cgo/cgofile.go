package ps2123cgo

// The only fmt reference in this cgo FILE is the fixable Sprint itself:
// the concatenation rewrite would orphan the import, and a cgo file's
// import block is never pruned, so the fix is withheld — the report
// stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import "fmt"

func cgoConcat(a, b string) string {
	return fmt.Sprint(a, b) // want `fmt\.Sprint over only plain strings inserts no separators and boxes every operand through fmt's reflection machinery; direct \+ concatenation builds the identical string`
}
