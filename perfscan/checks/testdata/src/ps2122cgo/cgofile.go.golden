package ps2122cgo

// The only fmt reference in this cgo FILE is the fixable Sprintf itself:
// the concatenation rewrite would orphan the import, and a cgo file's
// import block is never pruned, so the fix is withheld — the report
// stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import "fmt"

func cgoConcat(a, b string) string {
	return fmt.Sprintf("%s%s", a, b) // want `fmt\.Sprintf with a format of only %s verbs over plain strings boxes every argument and walks fmt's formatter state machine; direct \+ concatenation builds the identical string`
}
