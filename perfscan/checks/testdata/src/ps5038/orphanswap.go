package ps5038

import (
	"fmt"
	"os"
)

// The two fmt references in this FILE are the fixable calls themselves:
// applying both fixes orphans the fmt import, so its spec is swapped for
// "io" in place.
func orphanSwap(s string) {
	fmt.Fprintln(os.Stdout, s) // want `fmt\.Fprintln\(w, s\) on a single plain string pays fmt's interface boxing, pooled printer and format walk just to write s plus one newline; io\.WriteString\(w, s\+"\\n"\) writes the identical bytes with the same \(n, err\)`
	fmt.Fprintln(os.Stderr, s) // want `fmt\.Fprintln\(w, s\) on a single plain string pays fmt's interface boxing, pooled printer and format walk just to write s plus one newline; io\.WriteString\(w, s\+"\\n"\) writes the identical bytes with the same \(n, err\)`
}
