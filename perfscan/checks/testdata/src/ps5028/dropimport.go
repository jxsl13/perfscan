package ps5028

import (
	"fmt"
	"io"
)

// io is already imported and stays used; the sole fmt reference is the
// fixable call, so the fix drops the orphaned fmt spec.
func dropFmt(w io.Writer) {
	fmt.Fprintf(w, "bye\n") // want `fmt\.Fprintf with a verbless constant format and no operands pays fmt's pooled printer, format scan and intermediate buffer copy just to write the literal's bytes; io\.WriteString\(w, s\) hands them to w directly with the same \(n, err\)`
}
