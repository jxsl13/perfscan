package ps2042

import (
	"fmt"
	"io"
)

// The sole fmt reference is the fixable call, so the fix drops the
// orphaned fmt spec (the rewrite itself needs no import at all).
func dropFmt(w io.Writer, b []byte) {
	fmt.Fprintf(w, "%s", b) // want `fmt\.Fprintf\(w, "%s", b\) on a \[\]byte pays fmt's format parse, interface boxing and pooled-buffer copy just to hand the bytes to a single w\.Write; w\.Write\(b\) writes them directly with the same \(n, err\)`
}
