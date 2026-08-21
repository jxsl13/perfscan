package ps5038

import (
	"fmt"
	f "fmt"
	"os"
)

// Pathological: fmt imported under two names in one file. Rewriting this
// f.Fprintln would remove the only use of the f alias while fmt.* stays
// used via the plain name — the name-blind ref count cannot tell, so a
// fix would orphan the f spec ("imported as f and not used"). PS5038
// stays advisory.
func multiFmt(s string) {
	f.Fprintln(os.Stdout, s) // want `fmt\.Fprintln\(w, s\) on a single plain string pays fmt's interface boxing, pooled printer and format walk just to write s plus one newline; io\.WriteString\(w, s\+"\\n"\) writes the identical bytes with the same \(n, err\)`
	fmt.Fprint(os.Stdout, "keep the plain fmt name used")
}
