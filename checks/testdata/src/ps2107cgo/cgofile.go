package ps2107cgo

// The only fmt reference in this cgo FILE is the fixable Sprintf itself
// (strconv is already imported and used, so the fix would need no import
// edit): the rewrite would orphan the fmt import, and a cgo file's import
// block is never pruned, so the fix is withheld — the report stays
// advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import (
	"fmt"
	"strconv"
)

func cgoDecimal(i int) string {
	return fmt.Sprintf("%d", i) // want `fmt\.Sprintf of a single %d value boxes the argument and walks fmt's formatter state machine; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}

func cgoParse(s string) (int, error) {
	return strconv.Atoi(s)
}
