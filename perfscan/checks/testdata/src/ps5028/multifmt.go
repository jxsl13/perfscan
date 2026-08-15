package ps5028

import (
	"fmt"
	f "fmt"
	"os"
)

// Pathological: fmt imported under two names in one file. Rewriting this
// f.Fprintf would remove the only use of the f alias while fmt.* stays used
// via the plain name — the name-blind ref count cannot tell, so a fix would
// orphan the f spec ("imported as f and not used"). PS5028 stays advisory.
func multiFmt() {
	f.Fprintf(os.Stdout, "multi\n") // want `fmt\.Fprintf with a verbless constant format and no operands pays fmt's pooled printer, format scan and intermediate buffer copy just to write the literal's bytes; io\.WriteString\(w, s\) hands them to w directly with the same \(n, err\)`
	fmt.Fprintln(os.Stdout, "keep the plain fmt name used")
}
