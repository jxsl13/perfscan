package ps2017cgo

// The rewrite would need import surgery — adding strings and dropping
// the orphaned bytes — but a cgo file's import block is never edited, so
// the fix is withheld and the report stays advisory (the golden is
// identical).

// #include <stdlib.h>
import "C"

import (
	"bytes"
	"unicode"
)

func cgoMap(s string) string {
	return string(bytes.Map(unicode.ToUpper, []byte(s))) // want `string\(bytes\.Map\(f, \[\]byte\(s\)\)\) copies`
}
