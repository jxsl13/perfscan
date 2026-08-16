package ps2048

import (
	stdfmt "fmt"
	"os"
)

// An aliased fmt import still matches — the callee is resolved by type
// information — and the rewrite removes the alias's only reference, so
// the whole spec (alias included) is swapped for "io".
func aliasedFmt(a, b string) {
	stdfmt.Fprint(os.Stdout, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
}
