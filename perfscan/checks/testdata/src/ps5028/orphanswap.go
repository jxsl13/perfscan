package ps5028

import (
	"fmt"
	"os"
)

// The two fmt references in this FILE are the fixable calls themselves:
// applying both fixes orphans the fmt import, so its spec is swapped for
// "io" in place.
func orphanSwap() {
	fmt.Fprintf(os.Stdout, "ok\n")  // want `fmt\.Fprintf with a verbless constant format and no operands pays fmt's pooled printer, format scan and intermediate buffer copy just to write the literal's bytes; io\.WriteString\(w, s\) hands them to w directly with the same \(n, err\)`
	fmt.Fprintf(os.Stderr, "err\n") // want `fmt\.Fprintf with a verbless constant format and no operands pays fmt's pooled printer, format scan and intermediate buffer copy just to write the literal's bytes; io\.WriteString\(w, s\) hands them to w directly with the same \(n, err\)`
}
