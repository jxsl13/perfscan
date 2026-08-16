package ps2049

import (
	"fmt"
	xio "io"
)

// io is imported under an alias: the fix emits the bare io qualifier,
// which would not resolve here — advisory only.
func aliasedIo(w xio.Writer, a, b string) {
	fmt.Fprintln(w, a, b) // want `fmt\.Fprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's machinery; io\.WriteString\(w, a\+" "\+b\+"\\n"\) writes the identical bytes with the same \(n, err\)`
}
