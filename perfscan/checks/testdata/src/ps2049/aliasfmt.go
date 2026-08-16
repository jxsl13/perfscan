package ps2049

import (
	stdfmt "fmt"
	"os"
)

// An aliased fmt import still matches — the callee is resolved by type
// information — and the rewrite removes the alias's only reference, so
// the whole spec (alias included) is swapped for "io".
func aliasedFmt(a, b string) {
	stdfmt.Fprintln(os.Stdout, a, b) // want `fmt\.Fprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's machinery; io\.WriteString\(w, a\+" "\+b\+"\\n"\) writes the identical bytes with the same \(n, err\)`
}
