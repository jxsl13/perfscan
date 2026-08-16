package ps2048

import (
	"bytes"
	"fmt"
)

// io is NOT imported here: the fixable site's fix also inserts the io
// import at its sorted position; the fmt.Fprintln reference keeps fmt
// alive, so only the addition is needed.
func addImport(buf *bytes.Buffer, a, b string) {
	fmt.Fprint(buf, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
	fmt.Fprintln(buf)
}
