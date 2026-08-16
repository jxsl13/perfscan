package ps2049

import (
	"fmt"
	"io"
)

// io is already imported and stays used; the sole fmt reference is the
// fixable call, so the fix drops the orphaned fmt spec.
func dropFmt(w io.Writer, a, b string) {
	fmt.Fprintln(w, a, b) // want `fmt\.Fprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's machinery; io\.WriteString\(w, a\+" "\+b\+"\\n"\) writes the identical bytes with the same \(n, err\)`
}
