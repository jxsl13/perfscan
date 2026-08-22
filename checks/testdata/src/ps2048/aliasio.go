package ps2048

import (
	"fmt"
	xio "io"
)

// io is imported under an alias: the fix emits the bare io qualifier,
// which would not resolve here — advisory only.
func aliasedIo(w xio.Writer, a, b string) {
	fmt.Fprint(w, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}
