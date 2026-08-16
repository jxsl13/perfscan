package ps5038

import (
	"bytes"
	"fmt"
)

// io is NOT imported here: the fixable site's fix also inserts the io
// import at its sorted position; the fmt.Fprintf reference keeps fmt
// alive, so only the addition is needed.
func addImport(buf *bytes.Buffer, s string) {
	fmt.Fprintln(buf, s) // want `fmt\.Fprintln\(w, s\) on a single plain string pays fmt's interface boxing, pooled printer and format walk just to write s plus one newline; io\.WriteString\(w, s\+"\\n"\) writes the identical bytes with the same \(n, err\)`
	fmt.Fprintf(buf, "%d\n", buf.Len())
}
