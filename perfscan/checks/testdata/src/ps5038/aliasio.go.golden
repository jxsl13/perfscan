package ps5038

import (
	"fmt"
	xio "io"
)

// io is imported under an alias: the fix emits the bare io qualifier,
// which would not resolve here — advisory only.
func aliasedIo(w xio.Writer, s string) {
	fmt.Fprintln(w, s) // want `fmt\.Fprintln\(w, s\) on a single plain string pays fmt's interface boxing, pooled printer and format walk just to write s plus one newline; io\.WriteString\(w, s\+"\\n"\) writes the identical bytes with the same \(n, err\)`
}
