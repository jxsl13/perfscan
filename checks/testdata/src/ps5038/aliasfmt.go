package ps5038

import (
	stdfmt "fmt"
	"os"
)

// An aliased fmt import still matches — the callee is resolved by type
// information — and the rewrite removes the alias's only reference, so
// the whole spec (alias included) is swapped for "io".
func aliasedFmt(s string) {
	stdfmt.Fprintln(os.Stdout, s) // want `fmt\.Fprintln\(w, s\) on a single plain string pays fmt's interface boxing, pooled printer and format walk just to write s plus one newline; io\.WriteString\(w, s\+"\\n"\) writes the identical bytes with the same \(n, err\)`
}
