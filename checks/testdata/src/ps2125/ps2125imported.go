package ps2125

import "unicode/utf8"

// unicode/utf8 is already imported: the fix reuses it and must not add
// a duplicate import.
func alreadyImported(s string) bool {
	n := len([]rune(s)) // want `len\(\[\]rune\(s\)\) spells a rune count as a throwaway \[\]rune conversion; utf8\.RuneCountInString\(s\) is the direct, bit-identical count \(allocation-free on every toolchain\)`
	return n > 0 && utf8.ValidString(s)
}
