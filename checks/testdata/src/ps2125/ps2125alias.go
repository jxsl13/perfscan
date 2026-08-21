package ps2125

import u8 "unicode/utf8"

// unicode/utf8 is imported under an alias: the fix uses that alias.
func aliased(s string) bool {
	n := len([]rune(s)) // want `len\(\[\]rune\(s\)\) spells a rune count as a throwaway \[\]rune conversion; utf8\.RuneCountInString\(s\) is the direct, bit-identical count \(allocation-free on every toolchain\)`
	return n > 0 && u8.ValidString(s)
}
