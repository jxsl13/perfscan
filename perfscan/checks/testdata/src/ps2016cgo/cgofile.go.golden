package ps2016cgo

// The rewrite would need import surgery — adding strings and dropping
// the orphaned bytes — but a cgo file's import block is never edited, so
// the fix is withheld and the report stays advisory (the golden is
// identical).

// #include <stdlib.h>
import "C"

import "bytes"

func cgoTrim(s string) string {
	return string(bytes.TrimFunc([]byte(s), func(r rune) bool { return r == ' ' })) // want `string\(bytes\.TrimFunc\(\[\]byte\(s\), f\)\) copies`
}
