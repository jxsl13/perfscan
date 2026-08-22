package ps2118cgo

// The only io reference in this cgo FILE is the fixable WriteString
// itself: the w.Write(b) rewrite would orphan the import, and a cgo
// file's import block is never pruned, so the fix is withheld — the
// report stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import (
	"bytes"
	"io"
)

func cgoWrite(buf *bytes.Buffer, b []byte) {
	io.WriteString(buf, string(b)) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}
