package ps2129

import (
	"fmt"
	xio "io"
)

// io is imported under an alias: the fix emits the bare io qualifier,
// which would not resolve here — advisory only.
func aliasedIo(w xio.Writer, s string) {
	fmt.Fprintf(w, "%s", s) // want `fmt\.Fprintf\(w, "%s", s\) on a plain string pays fmt's format parse, interface boxing and reflection just to copy the bytes; io\.WriteString\(w, s\) writes them directly with the same \(n, err\)`
}
