package ps2049

import (
	"fmt"
	"os"
)

// The two fmt references in this FILE are the fixable calls themselves:
// applying both fixes orphans the fmt import, so its spec is swapped for
// "io" in place.
func orphanSwap(a, b string) {
	fmt.Fprintln(os.Stdout, a, b) // want `fmt\.Fprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's machinery; io\.WriteString\(w, a\+" "\+b\+"\\n"\) writes the identical bytes with the same \(n, err\)`
	fmt.Fprintln(os.Stderr, a, b, "!") // want `fmt\.Fprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's machinery; io\.WriteString\(w, a\+" "\+b\+"\\n"\) writes the identical bytes with the same \(n, err\)`
}
