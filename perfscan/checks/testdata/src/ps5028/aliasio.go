package ps5028

import (
	"fmt"
	xio "io"
)

// io is imported under an alias: the fix emits the bare io qualifier,
// which would not resolve here — advisory only.
func aliasedIo(w xio.Writer) {
	fmt.Fprintf(w, "aliased io\n") // want `fmt\.Fprintf with a verbless constant format and no operands pays fmt's pooled printer, format scan and intermediate buffer copy just to write the literal's bytes; io\.WriteString\(w, s\) hands them to w directly with the same \(n, err\)`
}
