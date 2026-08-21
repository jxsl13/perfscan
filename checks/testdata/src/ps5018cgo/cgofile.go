package ps5018cgo

// The rewrite would orphan the unicode import (this call holds the
// file's only unicode reference), but a cgo file's import block is
// never edited — the fix is withheld and the report stays advisory
// (the golden is identical).

// #include <stdlib.h>
import "C"

import (
	"strings"
	"unicode"
)

func cgoUpper(s string) string {
	return strings.Map(unicode.ToUpper, s) // want `strings\.Map\(unicode\.ToUpper, s\) pays`
}
