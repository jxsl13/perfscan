package ps2042

import (
	f "fmt"
	"fmt"
	"io"
)

// Legal but pathological: fmt imported under two names. The name-blind
// ref count cannot tell which spec a rewrite orphans, so every report in
// this file stays advisory.
func multi(w io.Writer, b []byte) {
	f.Fprintf(w, "%s", b)   // want `fmt\.Fprintf\(w, "%s", b\) on a \[\]byte pays fmt's format parse, interface boxing and pooled-buffer copy just to hand the bytes to a single w\.Write; w\.Write\(b\) writes them directly with the same \(n, err\)`
	fmt.Fprintf(w, "%s", b) // want `fmt\.Fprintf\(w, "%s", b\) on a \[\]byte pays fmt's format parse, interface boxing and pooled-buffer copy just to hand the bytes to a single w\.Write; w\.Write\(b\) writes them directly with the same \(n, err\)`
}
