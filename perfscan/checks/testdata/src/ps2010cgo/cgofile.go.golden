package ps2010cgo

// The only bytes reference in this cgo FILE is the fixable Equal call
// itself: the rewrite would orphan the import, and a cgo file's import
// block is never pruned, so the fix is withheld — the report stays
// advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import "bytes"

func cgoEq(a, b string) bool {
	return bytes.Equal([]byte(a), []byte(b)) // want `bytes\.Equal`
}
