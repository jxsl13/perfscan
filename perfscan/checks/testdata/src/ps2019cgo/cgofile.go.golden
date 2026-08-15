package ps2019cgo

// The only strings reference in this cgo FILE is the fixable call itself:
// the bytes.Contains rewrite would orphan the strings import, and a cgo
// file's import block is never edited, so the fix is withheld — the
// report stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import (
	"strings"
)

func cgoContains(b, sub []byte) bool {
	return strings.Contains(string(b), string(sub)) // want `strings\.Contains\(string\(b\), string\(sub\)\) allocates two throwaway string copies just to scan them; bytes\.Contains\(b, sub\) runs the same scan on the bytes directly with zero allocations`
}
