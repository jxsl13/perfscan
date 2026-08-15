package ps2018cgo

// The rewrite would need import surgery — adding strings and dropping
// the orphaned bytes — but a cgo file's import block is never edited, so
// the fix is withheld and the report stays advisory (the golden is
// identical).

// #include <stdlib.h>
import "C"

import "bytes"

func cgoPad(s string) string {
	return string(bytes.Repeat([]byte(s), 32)) // want `string\(bytes\.Repeat\(\[\]byte\(s\), n\)\) copies`
}
