package ps2138

import (
	bb "bytes"
	u8 "unicode/utf8"
)

// Both packages are imported under aliases: the qualifier match resolves
// bb to the stdlib bytes by import path, and the fix reuses the existing
// u8 alias for unicode/utf8.
func aliased(b []byte) bool {
	n := len(bb.Runes(b)) // want `len\(bb\.Runes\(b\)\) allocates a throwaway \[\]rune of every decoded rune just to count them; utf8\.RuneCount\(b\) is the direct, bit-identical count \(allocation-free\)`
	return n > 0 && u8.Valid(b) && bb.Equal(b, b)
}
