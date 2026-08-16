package ps2048

import (
	"fmt"
	f "fmt"
	"os"
)

// Pathological: fmt imported under two names in one file. Rewriting this
// f.Fprint would remove the only use of the f alias while fmt.* stays used
// via the plain name — the name-blind ref count cannot tell, so a fix would
// orphan the f spec ("imported as f and not used"). PS2048 stays advisory.
func multiFmt(a, b string) {
	f.Fprint(os.Stdout, a, b) // want `fmt\.Fprint over only plain strings inserts no separators, boxes every operand and walks fmt's reflection printer just to write their concatenation; io\.WriteString\(w, a\+b\+\.\.\.\) writes the identical bytes with the same \(n, err\)`
	fmt.Fprintln(os.Stdout, "keep the plain fmt name used")
}
