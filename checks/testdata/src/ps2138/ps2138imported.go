package ps2138

import (
	"bytes"
	"unicode/utf8"
)

// unicode/utf8 is already imported: the fix reuses it and must not add
// a duplicate import.
func alreadyImported(b []byte) bool {
	n := len(bytes.Runes(b)) // want `len\(bytes\.Runes\(b\)\) allocates a throwaway \[\]rune of every decoded rune just to count them; utf8\.RuneCount\(b\) is the direct, bit-identical count \(allocation-free\)`
	return n > 0 && utf8.Valid(b) && bytes.Equal(b, b)
}
