package ps5005cgo

// The rewrite would need import surgery — adding strings and dropping
// the orphaned bytes — but a cgo file's import block is never edited, so
// the fix is withheld and the report stays advisory (the golden is
// identical).

// #include <stdlib.h>
import "C"

import "bytes"

func cgoTrim(s string) string {
	return string(bytes.Trim([]byte(s), " \t")) // want `string\(bytes\.Trim\(\[\]byte\(s\), cutset\)\) copies`
}
