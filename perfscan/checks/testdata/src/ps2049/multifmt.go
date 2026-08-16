package ps2049

import (
	"fmt"
	f "fmt"
	"os"
)

// Pathological: fmt imported under two names in one file. Rewriting this
// f.Fprintln would remove the only use of the f alias while fmt.* stays
// used via the plain name — the name-blind ref count cannot tell, so a
// fix would orphan the f spec ("imported as f and not used"). PS2049
// stays advisory.
func multiFmt(a, b string) {
	f.Fprintln(os.Stdout, a, b) // want `fmt\.Fprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's machinery; io\.WriteString\(w, a\+" "\+b\+"\\n"\) writes the identical bytes with the same \(n, err\)`
	fmt.Fprint(os.Stdout, "keep the plain fmt name used")
}
