package ps2034cgo

// The only fmt reference in this cgo FILE is the fixable Sprintf itself:
// the concatenation rewrite would orphan the import, and a cgo file's
// import block is never pruned, so the fix is withheld — the report
// stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import "fmt"

func cgoSplice(a, b string) string {
	return fmt.Sprintf("a=%s b=%s", a, b) // want `fmt\.Sprintf splicing plain strings into literal text boxes every argument and walks fmt's formatter state machine; \+ concatenation with the literal segments builds the identical string`
}
