package ps2129

import (
	"fmt"
	"io"
)

// io is already imported and stays used; the sole fmt reference is the
// fixable call, so the fix drops the orphaned fmt spec.
func dropFmt(w io.Writer, s string) {
	fmt.Fprint(w, s) // want `fmt\.Fprint\(w, s\) on a single plain string pays fmt's interface boxing and reflection just to copy the bytes; io\.WriteString\(w, s\) writes them directly with the same \(n, err\)`
}
