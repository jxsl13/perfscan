package ps5039

import u8 "unicode/utf8"

// An existing aliased unicode/utf8 import is reused verbatim as the
// qualifier — no import edit in this file.
func aliased(dst []byte, r rune) []byte {
	return append(dst, string(r)...) // want `append\(dst, string\(r\)\.\.\.\) UTF-8-encodes r into a throwaway string that append copies a second time; utf8\.AppendRune\(dst, r\) encodes straight into dst`
}

var _ = u8.UTFMax
