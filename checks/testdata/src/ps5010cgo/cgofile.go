package ps5010cgo

// The rewrite would need import surgery — adding strings and dropping
// the orphaned bytes — but a cgo file's import block is never edited, so
// the fix is withheld and the report stays advisory (the golden is
// identical).

// #include <stdlib.h>
import "C"

import "bytes"

func cgoUpper(s string) string {
	return string(bytes.ToUpper([]byte(s))) // want `string\(bytes\.ToUpper\(\[\]byte\(s\)\)\) copies`
}
