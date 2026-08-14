package ps3004cgo

// The only bytes reference in this cgo FILE is the fixable call itself:
// the strings.Contains rewrite would orphan the bytes import, and a cgo
// file's import block is never edited, so the fix is withheld — the
// report stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import (
	"bytes"
)

func cgoContains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub)) // want `bytes\.Contains\(\[\]byte\(s\), \[\]byte\(sub\)\) allocates two throwaway \[\]byte copies just to scan them; strings\.Contains\(s, sub\) runs the same scan on the string bytes directly with zero allocations`
}
