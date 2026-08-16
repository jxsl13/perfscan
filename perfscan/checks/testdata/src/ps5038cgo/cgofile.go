package ps5038cgo

// The only fmt reference in this cgo FILE is the fixable Fprintln
// itself: the io.WriteString rewrite would orphan the import (and would
// also need to add "io"), and a cgo file's import block is never
// edited, so the fix is withheld — the report stays advisory and the
// golden is identical.

// #include <stdlib.h>
import "C"

import "fmt"

import "os"

func cgoWrite(s string) {
	fmt.Fprintln(os.Stdout, s) // want `fmt\.Fprintln\(w, s\) on a single plain string pays fmt's interface boxing, pooled printer and format walk just to write s plus one newline; io\.WriteString\(w, s\+"\\n"\) writes the identical bytes with the same \(n, err\)`
}
