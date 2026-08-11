package ps2118

// The only io reference in this FILE is the fixable WriteString itself:
// the w.Write(b) rewrite would orphan the import, so advisory only.

import (
	"bytes"
	"io"
)

func orphanWrite(buf *bytes.Buffer, b []byte) {
	io.WriteString(buf, string(b)) // want `io\.WriteString\(w, string\(b\)\) allocates a string copy of the byte slice just to write it; w\.Write\(b\) writes the bytes directly`
}
