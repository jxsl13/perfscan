package ps2129

import (
	"bytes"
	"fmt"
)

// io is NOT imported here: the fixable site's fix also inserts the io
// import at its sorted position; the fmt.Fprintln reference keeps fmt
// alive, so only the addition is needed.
func addImport(buf *bytes.Buffer, s string) {
	fmt.Fprintf(buf, "%v", s) // want `fmt\.Fprintf\(w, "%v", s\) on a plain string pays fmt's format parse, interface boxing and reflection just to copy the bytes; io\.WriteString\(w, s\) writes them directly with the same \(n, err\)`
	fmt.Fprintln(buf)
}
