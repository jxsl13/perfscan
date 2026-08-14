package ps5011cgo

// The rewrite would need import surgery — adding strings and dropping
// the orphaned bytes — but a cgo file's import block is never edited, so
// the fix is withheld and the report stays advisory (the golden is
// identical).

// #include <stdlib.h>
import "C"

import "bytes"

func cgoReplace(s string) string {
	return string(bytes.ReplaceAll([]byte(s), []byte("\t"), []byte(" "))) // want `string\(bytes\.ReplaceAll\(\[\]byte\(s\), \[\]byte\(old\), \[\]byte\(new\)\)\) copies`
}
