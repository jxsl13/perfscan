package ps2042

import (
	stdfmt "fmt"
	"os"
)

// An aliased fmt import still matches — the callee is resolved by type
// information — and the rewrite removes the alias's only reference, so
// the whole spec (alias included) is dropped.
func aliased(b []byte) {
	stdfmt.Fprintf(os.Stdout, "%s", b) // want `fmt\.Fprintf\(w, "%s", b\) on a \[\]byte pays fmt's format parse, interface boxing and pooled-buffer copy just to hand the bytes to a single w\.Write; w\.Write\(b\) writes them directly with the same \(n, err\)`
}
