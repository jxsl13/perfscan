package ps2044cgo

// The only fmt reference in this cgo FILE is the fixable Appendf itself:
// the append-chain rewrite would orphan the import, and a cgo file's
// import block is never pruned, so the fix is withheld — the report
// stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import "fmt"

func cgoSplice(buf []byte, k, v string) []byte {
	return fmt.Appendf(buf, "%s=%s;", k, v) // want `fmt\.Appendf splicing plain strings into literal text`
}
