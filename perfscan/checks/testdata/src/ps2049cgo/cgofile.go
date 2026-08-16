package ps2049cgo

// The only fmt reference in this cgo FILE is the fixable Fprintln
// itself: the io.WriteString rewrite would orphan the import (and would
// also need to add "io"), and a cgo file's import block is never
// edited, so the fix is withheld — the report stays advisory and the
// golden is identical.

// #include <stdlib.h>
import "C"

import "fmt"

import "os"

func cgoWrite(a, b string) {
	fmt.Fprintln(os.Stdout, a, b) // want `fmt\.Fprintln over only plain strings writes exactly one space between operands and one trailing newline, boxing every operand through fmt's machinery; io\.WriteString\(w, a\+" "\+b\+"\\n"\) writes the identical bytes with the same \(n, err\)`
}
