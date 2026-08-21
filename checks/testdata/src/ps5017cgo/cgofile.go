package ps5017cgo

// The rewrite would orphan the unicode import (this call holds the
// file's only unicode reference), but a cgo file's import block is
// never edited — the fix is withheld and the report stays advisory
// (the golden is identical).

// #include <stdlib.h>
import "C"

import (
	"bytes"
	"unicode"
)

func cgoUpper(b []byte) []byte {
	return bytes.Map(unicode.ToUpper, b) // want `bytes\.Map\(unicode\.ToUpper, b\) pays`
}
