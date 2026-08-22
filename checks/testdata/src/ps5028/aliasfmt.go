package ps5028

import (
	stdfmt "fmt"
	"os"
)

// An aliased fmt import still matches — the callee is resolved by type
// information — and the rewrite removes the alias's only reference, so
// the whole spec (alias included) is swapped for "io".
func aliasedFmt() {
	stdfmt.Fprintf(os.Stdout, "aliased\n") // want `fmt\.Fprintf with a verbless constant format and no operands pays fmt's pooled printer, format scan and intermediate buffer copy just to write the literal's bytes; io\.WriteString\(w, s\) hands them to w directly with the same \(n, err\)`
}
