package ps2129

import (
	"fmt"
	f "fmt"
	"os"
)

// Pathological: fmt imported under two names in one file. Rewriting this
// f.Fprintf would remove the only use of the f alias while fmt.* stays used
// via the plain name — the name-blind ref count cannot tell, so a fix would
// orphan the f spec ("imported as f and not used"). PS2129 stays advisory.
func multiFmt(s string) {
	f.Fprintf(os.Stdout, "%s", s) // want `fmt\.Fprintf\(w, "%s", s\) on a plain string pays fmt's format parse, interface boxing and reflection just to copy the bytes; io\.WriteString\(w, s\) writes them directly with the same \(n, err\)`
	fmt.Fprintln(os.Stdout, "keep the plain fmt name used")
}
