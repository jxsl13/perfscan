package ps2138

// The only bytes reference in this FILE is the fixable call itself: the
// rewrite orphans the import, and the fix pipeline prunes the now-unused
// bytes import afterwards, so the fix applies (non-cgo file) and adds
// the unicode/utf8 import it needs.

import "bytes"

func orphanCount(b []byte) int {
	return len(bytes.Runes(b)) // want `len\(bytes\.Runes\(b\)\) allocates a throwaway \[\]rune of every decoded rune just to count them; utf8\.RuneCount\(b\) is the direct, bit-identical count \(allocation-free\)`
}
